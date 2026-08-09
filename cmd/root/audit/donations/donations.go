package donations

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
	donationstore "boardfund/service/donations/store"
	"boardfund/service/finance"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

func DonationsAuditCmd(runConfig *root.RunConfig) *cobra.Command {
	return &cobra.Command{
		Use: "donations",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logging.New("reconcile-donations")

			return logging.Job(cmd.Context(), logger, "reconcile-donations",
				func(ctx context.Context) ([]slog.Attr, error) {
					return nil, reconcile(ctx, runConfig, logger)
				})
		},
	}
}

// reconcile is the body of the command, lifted out of the RunE so the job
// wrapper can bracket all of it.
//
// Including the part that opens the database and reads the AWS configuration.
// Those are how a scheduled run ends before reaching any code that logs, which
// is indistinguishable from the schedule not having fired -- and telling those
// two apart is the whole reason the started line exists.
func reconcile(ctx context.Context, runConfig *root.RunConfig, logger *slog.Logger) error {
	dbURI := fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s",
		runConfig.PGUser, runConfig.PGPass, runConfig.PGHost, runConfig.PGPort, runConfig.PGDB,
	)

	tokenClient := token.NewClient(
		runConfig.PayPal.ClientID,
		runConfig.PayPal.ClientSecret,
		runConfig.PayPal.BaseURL,
	)
	tokenStore := token.NewStore(tokenClient)
	paypalClient := paypal.NewClient(tokenStore, logger, runConfig.PayPal.BaseURL)
	paypalService := paypal.NewPaypal(paypalClient, runConfig.PayPal.ProductID)

	pool, err := pg.GetDBPool(dbURI)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool: %w", err)
	}

	defaultConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-west-2"))
	if err != nil {
		return err
	}

	s3Client := s3.NewFromConfig(defaultConfig)
	donationsPaymentsS3 := aws.NewAWSS3(s3Client, logger, runConfig.DonationsPaymentsReportsS3Bucket)

	donationStore := donationstore.NewDonationStore(pool)
	fundEvents := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

	financeService := finance.NewFinanceService(
		donationStore, paypalService, donationsPaymentsS3, fundEvents, runConfig.ReportTypes, logger,
	)

	err = financeService.RunRecurringDonationReconciliation(ctx)
	if err != nil {
		return fmt.Errorf("failed to reconcile recurring donations: %w", err)
	}

	err = financeService.RunOneTimeDonationReconciliation(ctx)
	if err != nil {
		return fmt.Errorf("failed to reconcile one-time donations: %w", err)
	}

	return nil
}
