package payout

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"boardfund/cmd/root"
	"boardfund/service/payouts"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func planCmd(runConfig *root.RunConfig) *cobra.Command {
	var (
		fundIDStr   string
		amountCents int32
		payoutDate  string
		description string
		notes       string
		autoApprove bool
	)

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "build a payout batch for a fund and leave it awaiting approval",
		RunE: func(cmd *cobra.Command, args []string) error {
			fundID, err := uuid.Parse(fundIDStr)
			if err != nil {
				return fmt.Errorf("invalid --fund: %w", err)
			}

			if amountCents <= 0 {
				return errors.New("--amount-cents must be greater than zero")
			}

			date := time.Now()
			if payoutDate != "" {
				date, err = time.Parse("2006-01-02", payoutDate)
				if err != nil {
					return fmt.Errorf("invalid --date, want YYYY-MM-DD: %w", err)
				}
			}

			d, err := build(runConfig)
			if err != nil {
				return err
			}

			batch, err := d.service.PlanBatch(cmd.Context(), payouts.PlanBatch{
				FundID:          fundID,
				PayoutDate:      date,
				AmountCents:     amountCents,
				Description:     description,
				Notes:           notes,
				RequireApproval: !autoApprove,
			})
			if err != nil {
				if errors.Is(err, payouts.ErrNoEnrollments) {
					fmt.Println("no eligible enrollments; nothing to pay this period")

					return nil
				}

				return err
			}

			printBatch(*batch)

			return nil
		},
	}

	cmd.Flags().StringVar(&fundIDStr, "fund", "", "fund UUID (required)")
	cmd.Flags().Int32Var(&amountCents, "amount-cents", 0, "amount paid to each enrollee, in cents (required)")
	cmd.Flags().StringVar(&payoutDate, "date", "", "payout date as YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&description, "description", "", "description recorded on the batch and sent to recipients")
	cmd.Flags().StringVar(&notes, "notes", "", "internal notes, not sent to recipients")
	cmd.Flags().BoolVar(&autoApprove, "no-approval", false, "skip the treasurer gate and create the batch ready to submit")

	_ = cmd.MarkFlagRequired("fund")
	_ = cmd.MarkFlagRequired("amount-cents")

	return cmd
}

func listCmd(runConfig *root.RunConfig) *cobra.Command {
	var (
		fundIDStr string
		pending   bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "list payout batches",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := build(runConfig)
			if err != nil {
				return err
			}

			var batches []payouts.Batch

			switch {
			case pending:
				batches, err = d.service.GetBatchesAwaitingApproval(cmd.Context())
			case fundIDStr != "":
				fundID, errParse := uuid.Parse(fundIDStr)
				if errParse != nil {
					return fmt.Errorf("invalid --fund: %w", errParse)
				}

				batches, err = d.service.GetBatchesForFund(cmd.Context(), fundID)
			default:
				return errors.New("specify --fund or --pending")
			}

			if err != nil {
				return err
			}

			if len(batches) == 0 {
				fmt.Println("no batches")

				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "BATCH ID\tSTATUS\tAMOUNT\tPAYEES\tPAYOUT DATE\tAPPROVAL DEADLINE")

			for _, b := range batches {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
					b.ID,
					b.Status,
					dollars(b.AmountCents),
					b.NumEnrollments,
					b.PayoutDate.Format("2006-01-02"),
					deadline(b),
				)
			}

			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&fundIDStr, "fund", "", "list batches for this fund UUID")
	cmd.Flags().BoolVar(&pending, "pending", false, "list every batch awaiting approval, across funds")

	return cmd
}

func showCmd(runConfig *root.RunConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <batch-id>",
		Short: "show a batch and its individual payouts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			batchID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid batch id: %w", err)
			}

			d, err := build(runConfig)
			if err != nil {
				return err
			}

			batch, err := d.service.GetBatchByID(cmd.Context(), batchID)
			if err != nil {
				return err
			}

			printBatch(*batch)

			items, err := d.service.GetPayoutsForBatch(cmd.Context(), batchID)
			if err != nil {
				return err
			}

			fmt.Println()

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PAYOUT ID\tSTATUS\tAMOUNT\tFEE\tDESTINATION")

			for _, item := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					item.ID,
					item.Status,
					dollars(item.AmountCents),
					dollars(item.ProviderFeeCents),
					item.DestinationEmail,
				)
			}

			return w.Flush()
		},
	}

	return cmd
}

func approveCmd(runConfig *root.RunConfig) *cobra.Command {
	var approverStr string

	cmd := &cobra.Command{
		Use:   "approve <batch-id>",
		Short: "approve a batch, making it eligible for submission",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			batchID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid batch id: %w", err)
			}

			approver, err := uuid.Parse(approverStr)
			if err != nil {
				return fmt.Errorf("invalid --approver, want a member UUID: %w", err)
			}

			d, err := build(runConfig)
			if err != nil {
				return err
			}

			batch, err := d.service.ApproveBatch(cmd.Context(), batchID, approver)
			if err != nil {
				return err
			}

			printBatch(*batch)

			return nil
		},
	}

	cmd.Flags().StringVar(&approverStr, "approver", "", "member UUID of the approving treasurer (required)")
	_ = cmd.MarkFlagRequired("approver")

	return cmd
}

func rejectCmd(runConfig *root.RunConfig) *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "reject <batch-id>",
		Short: "reject a batch awaiting approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			batchID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid batch id: %w", err)
			}

			d, err := build(runConfig)
			if err != nil {
				return err
			}

			batch, err := d.service.RejectBatch(cmd.Context(), batchID, reason)
			if err != nil {
				return err
			}

			printBatch(*batch)

			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "why the batch was rejected")

	return cmd
}

func submitCmd(runConfig *root.RunConfig) *cobra.Command {
	var confirm bool

	cmd := &cobra.Command{
		Use:   "submit <batch-id>",
		Short: "send an approved batch to the payments provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			batchID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid batch id: %w", err)
			}

			d, err := build(runConfig)
			if err != nil {
				return err
			}

			batch, err := d.service.GetBatchByID(cmd.Context(), batchID)
			if err != nil {
				return err
			}

			// This is the step that moves real money, so it will not run from a
			// half-typed command line.
			if !confirm {
				fmt.Printf(
					"about to send %s to %d recipients for fund %s\nre-run with --confirm to proceed\n",
					dollars(batch.AmountCents), batch.NumEnrollments, batch.FundID,
				)

				return nil
			}

			submitted, err := d.service.SubmitBatch(cmd.Context(), batchID)
			if err != nil {
				return err
			}

			printBatch(*submitted)

			return nil
		},
	}

	cmd.Flags().BoolVar(&confirm, "confirm", false, "actually send the batch; without this the command only reports what would be sent")

	return cmd
}

func sweepCmd(runConfig *root.RunConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "sweep",
		Short: "cancel batches whose approval window expired and send reminders",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := build(runConfig)
			if err != nil {
				return err
			}

			return d.service.RunApprovalSweep(cmd.Context())
		},
	}
}

func reconcileCmd(runConfig *root.RunConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "reconcile <batch-id>",
		Short: "poll the provider for a submitted batch and write back item status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			batchID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid batch id: %w", err)
			}

			d, err := build(runConfig)
			if err != nil {
				return err
			}

			err = d.service.ReconcileBatch(cmd.Context(), batchID)
			if err != nil {
				return err
			}

			batch, err := d.service.GetBatchByID(cmd.Context(), batchID)
			if err != nil {
				return err
			}

			printBatch(*batch)

			return nil
		},
	}
}

func printBatch(b payouts.Batch) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "batch\t%s\n", b.ID)
	fmt.Fprintf(w, "fund\t%s\n", b.FundID)
	fmt.Fprintf(w, "status\t%s\n", b.Status)
	fmt.Fprintf(w, "amount\t%s across %d payees\n", dollars(b.AmountCents), b.NumEnrollments)
	fmt.Fprintf(w, "payout date\t%s\n", b.PayoutDate.Format("2006-01-02"))
	fmt.Fprintf(w, "sender batch id\t%s\n", b.SenderBatchID)

	if b.ProviderBatchID != "" {
		fmt.Fprintf(w, "provider batch id\t%s\n", b.ProviderBatchID)
	}

	if b.ApprovalDeadline != nil {
		fmt.Fprintf(w, "approval deadline\t%s\n", deadline(b))
	}

	if b.ApprovedBy != nil && b.ApprovedAt != nil {
		fmt.Fprintf(w, "approved\t%s by %s\n", b.ApprovedAt.Format(time.RFC3339), b.ApprovedBy)
	}

	if b.FailureReason != "" {
		fmt.Fprintf(w, "failure reason\t%s\n", b.FailureReason)
	}

	_ = w.Flush()
}

func deadline(b payouts.Batch) string {
	if b.ApprovalDeadline == nil {
		return "-"
	}

	remaining := time.Until(*b.ApprovalDeadline).Truncate(time.Minute)
	if remaining <= 0 {
		return b.ApprovalDeadline.Format(time.RFC3339) + " (expired)"
	}

	return fmt.Sprintf("%s (in %s)", b.ApprovalDeadline.Format(time.RFC3339), remaining)
}

// dollars renders cents as currency. The sign is handled explicitly because Go's
// % keeps it, so -125 would otherwise print as "$-1.-25". Provider fees arrive
// negative on refunds and reversals.
func dollars(cents int32) string {
	amount := int64(cents)

	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}

	return fmt.Sprintf("%s$%d.%02d", sign, amount/100, amount%100)
}
