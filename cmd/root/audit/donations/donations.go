package donations

import (
	"boardfund/cmd/root"
	"boardfund/paypal"
	"boardfund/paypal/token"
	"boardfund/pg"
	donationstore "boardfund/service/donations/store"
	"boardfund/service/finance"
	"fmt"
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

			donationStore := donationstore.NewDonationStore(pool)

			financeService := finance.NewFinanceService(donationStore, paypalService, logger)
			err = financeService.RunDonationReconciliation(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to reconcile donations: %w", err)
			}

			return nil
		},
	}
}
