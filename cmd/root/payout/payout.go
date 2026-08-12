package payout

import (
	"fmt"
	"log/slog"

	"boardfund/cmd/root"
	"boardfund/logging"
	"boardfund/paypal"
	"boardfund/paypal/token"
	"boardfund/pg"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"
	"boardfund/service/payouts"
	payoutstore "boardfund/service/payouts/store"

	"github.com/spf13/cobra"
)

// PayoutCmd groups the payout lifecycle. Each stage is a separate command on
// purpose: planning, approving and submitting are distinct decisions, and a single
// "do the payout" command would collapse the approval gate into an implementation
// detail.
func PayoutCmd(runConfig *root.RunConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "payout",
		Short: "plan, approve and submit fund payouts",
	}

	cmd.AddCommand(
		planCmd(runConfig),
		listCmd(runConfig),
		showCmd(runConfig),
		approveCmd(runConfig),
		rejectCmd(runConfig),
		submitCmd(runConfig),
		sweepCmd(runConfig),
		reconcileCmd(runConfig),
		planDueCmd(runConfig),
		submitApprovedCmd(runConfig),
		reconcilePendingCmd(runConfig),
	)

	return cmd
}

type deps struct {
	service *payouts.PayoutService
	logger  *slog.Logger
}

// build wires the payout service. Deliberately constructed per-command rather than
// shared: these run as one-shot CLI invocations, and a connection pool held open
// across an interactive approval session buys nothing.
func build(runConfig *root.RunConfig) (*deps, error) {
	logger := logging.New("payout")

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
	paypalService := paypal.NewPaypal(paypalClient)

	store := payoutstore.NewPayoutStore(pool)
	fundEvents := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

	// No notifier wired yet: reminders are logged by the sweep until a delivery
	// channel exists. A nil notifier is handled by the service.
	service := payouts.NewPayoutService(
		store,
		paypalService,
		nil,
		fundEvents,
		runConfig.PayoutApprovalWindow,
		runConfig.PayoutReminderWindow,
		logger,
	)

	return &deps{service: service, logger: logger}, nil
}
