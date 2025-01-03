package donations

import (
	"boardfund/aws"
	"boardfund/cmd/root"
	"boardfund/paypal"
	"boardfund/paypal/token"
	"boardfund/pg"
	donationstore "boardfund/service/donations/store"
	"boardfund/service/finance"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
	"log/slog"
	"os"
)

func DonationsAuditCmd(runConfig *root.RunConfig) *cobra.Command {
	return &cobra.Command{
		Use: "donations",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURI := fmt.Sprintf(
				"postgresql://%s:%s@%s:%s/%s",
				runConfig.PGUser, runConfig.PGPass, runConfig.PGHost, runConfig.PGPort, runConfig.PGDB,
			)

			logHandler := slog.NewJSONHandler(os.Stdout, nil)
			logger := slog.New(logHandler)

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

			defaultConfig, err := config.LoadDefaultConfig(cmd.Context(), config.WithRegion("us-west-2"))
			if err != nil {
				return err
			}

			s3Client := s3.NewFromConfig(defaultConfig)
			donationsPaymentsS3 := aws.NewAWSS3(s3Client, logger, runConfig.DonationsPaymentsReportsS3Bucket)

			donationStore := donationstore.NewDonationStore(pool)

			financeService := finance.NewFinanceService(donationStore, paypalService, donationsPaymentsS3, runConfig.ReportTypes, logger)
			err = financeService.RunRecurringDonationReconciliation(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to reconcile recurring donations: %w", err)
			}

			err = financeService.RunOneTimeDonationReconciliation(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to reconcile one-time donations: %w", err)
			}

			return nil
		},
	}
}
