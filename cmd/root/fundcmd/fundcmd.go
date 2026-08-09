// Package fundcmd holds the scheduled jobs that act on funds themselves, as
// opposed to the payout lifecycle that runs on top of them.
package fundcmd

import (
	"context"
	"fmt"
	"log/slog"

	"boardfund/aws"
	"boardfund/cmd/root"
	"boardfund/logging"
	"boardfund/paypal"
	"boardfund/paypal/token"
	"boardfund/pg"
	"boardfund/service/donations"
	donationstore "boardfund/service/donations/store"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

func FundCmd(runConfig *root.RunConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "funds",
		Short: "scheduled jobs that act on funds",
	}

	cmd.AddCommand(closeExpiredCmd(runConfig))

	return cmd
}

// closeExpiredCmd must run after the payout planner on any day a fund expires.
// Closing a fund stops new batches being planned for it, so the reverse order
// silently drops the fund's final payout.
func closeExpiredCmd(runConfig *root.RunConfig) *cobra.Command {
	var confirm bool

	cmd := &cobra.Command{
		Use:   "close-expired",
		Short: "close every fund whose end date has passed",
		Long: "Deactivates expired funds and cancels their recurring subscriptions " +
			"at the provider. Without --confirm the command only reports what it " +
			"would close.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return logging.Job(cmd.Context(), logging.New("funds"), "close-expired",
				func(ctx context.Context) ([]slog.Attr, error) {
					service, err := build(runConfig)
					if err != nil {
						return nil, err
					}

					// Cancelling subscriptions at the provider is not reversible from
					// here, so the dry run is the default -- the same shape as submit.
					if !confirm {
						expired, errList := service.ListExpiredOpenFunds(ctx)
						if errList != nil {
							return nil, errList
						}

						if len(expired) == 0 {
							fmt.Println("no expired funds are still open")

							return []slog.Attr{slog.Bool("dry_run", true), slog.Int("would_close", 0)}, nil
						}

						fmt.Printf("would close %d fund(s):\n", len(expired))
						for _, fund := range expired {
							fmt.Printf("  %s  %s\n", fund.ID, fund.Name)
						}

						fmt.Println("\nre-run with --confirm to close them")

						// Said on the line, because a cron whose --confirm was dropped
						// looks healthy: it runs daily, it finds funds, and it closes
						// none of them.
						return []slog.Attr{
							slog.Bool("dry_run", true),
							slog.Int("would_close", len(expired)),
						}, nil
					}

					closed, err := service.CloseExpiredFunds(ctx)
					if err != nil {
						return nil, err
					}

					fmt.Printf("closed %d fund(s)\n", closed)

					return []slog.Attr{slog.Int("closed", closed)}, nil
				})
		},
	}

	cmd.Flags().BoolVar(&confirm, "confirm", false, "actually close the funds; without this the command only reports what would be closed")

	return cmd
}

// build wires the donation service for a one-shot CLI run, matching how the
// payout commands construct theirs.
func build(runConfig *root.RunConfig) (*donations.DonationService, error) {
	logger := logging.New("funds")

	dbURI := fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s",
		runConfig.PGUser, runConfig.PGPass, runConfig.PGHost, runConfig.PGPort, runConfig.PGDB,
	)

	pool, err := pg.GetDBPool(dbURI)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgx pool: %w", err)
	}

	tokenClient := token.NewClient(
		runConfig.PayPal.ClientID,
		runConfig.PayPal.ClientSecret,
		runConfig.PayPal.BaseURL,
	)
	tokenStore := token.NewStore(tokenClient)
	paypalClient := paypal.NewClient(tokenStore, logger, runConfig.PayPal.BaseURL)
	paypalService := paypal.NewPaypal(paypalClient, runConfig.PayPal.ProductID)

	// Reports are an S3 concern the closer never reaches, but the service needs a
	// document store to construct.
	defaultConfig, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-west-2"))
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(defaultConfig)
	documentStorage := aws.NewAWSS3(s3Client, logger, "")
	// The CLI never touches a fund picture, but the service it builds is the same
	// service, and a nil here would be a panic waiting for whoever adds a command
	// that does.
	fundImages := aws.NewFundImages(s3Client, runConfig.FundImagesS3Bucket, logger)

	store := donationstore.NewDonationStore(pool)
	fundEvents := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

	return donations.NewDonationService(
		store, documentStorage, fundImages, paypalService, fundEvents, runConfig.ReportTypes, logger,
	), nil
}
