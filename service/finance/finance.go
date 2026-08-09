package finance

import (
	"boardfund/service/donations"
	"boardfund/service/fundevents"
	"context"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"strings"
	"time"
)

type AuditDonation struct {
	Active                 bool
	ProviderSubscriptionID string
	FirstName              string
	LastName               string
}

type ProviderTransaction struct {
	ProviderPaymentID string
	Date              time.Time
	Status            string
	AmountCents       int32
	FeeCents          int32
}

type ReportInfo struct {
	FundID uuid.UUID
	Date   time.Time
	Type   string
}

// Audit is a fund's payments and what reconciliation last saw at the provider.
//
// Read from the database rather than from a stored report. It used to be a CSV
// per fund per day in S3, so the page could only show a fund as it stood when
// some past run wrote a file, and only for the days a run had happened. Now it
// answers for the payments as they are.
type Audit struct {
	FundID   uuid.UUID
	FundName string
	// Date is when the page was generated, kept so the view still has something
	// to say it is showing. It is no longer the date of a stored report.
	Date     time.Time
	Payments []AuditPayment
}

// AuditVerdict is what reconciliation makes of one payment.
type AuditVerdict string

const (
	// AuditUnchecked means reconciliation has not reached this payment. Distinct
	// from agreeing with the provider, and the old report could not tell them
	// apart: both were blank columns.
	AuditUnchecked AuditVerdict = "unchecked"
	// AuditOK means the provider reported it complete, for the amount we hold.
	AuditOK AuditVerdict = "ok"
	// AuditAmountMismatch is the finding worth having. The provider took a
	// different amount than the one the fund is counting on.
	AuditAmountMismatch AuditVerdict = "amount mismatch"
	// AuditNotSettled means the provider knows the payment and does not call it
	// complete.
	AuditNotSettled AuditVerdict = "not settled"
	// AuditMissingAtProvider means the provider returned nothing for it. Usually
	// only that the transaction is too recent to have appeared in reporting, which
	// lags by hours, so it is reported as unknown rather than as an error.
	AuditMissingAtProvider AuditVerdict = "not found at provider"
)

type AuditPayment struct {
	DonationID        uuid.UUID
	PaymentID         uuid.UUID
	ProviderPaymentID string
	DonorName         string
	Recurring         bool

	AmountCents    int32
	RefundedCents  int32
	FeeAmountCents int32

	ProviderStatus      string
	ProviderAmountCents int32
	ReconciledAt        *time.Time

	Created time.Time
}

// Verdict is derived rather than stored, so a change to what counts as a problem
// applies to every payment already on record instead of only to those a later run
// happens to revisit.
func (a AuditPayment) Verdict() AuditVerdict {
	if a.ReconciledAt == nil {
		return AuditUnchecked
	}

	if a.ProviderStatus == "" {
		return AuditMissingAtProvider
	}

	if !strings.EqualFold(a.ProviderStatus, "COMPLETED") {
		return AuditNotSettled
	}

	if a.ProviderAmountCents != a.AmountCents {
		return AuditAmountMismatch
	}

	return AuditOK
}

// NeedsAttention marks the rows a person should look at. Unchecked is not one of
// them: it means the job has not got there, not that anything is wrong.
func (a AuditPayment) NeedsAttention() bool {
	switch a.Verdict() {
	case AuditAmountMismatch, AuditNotSettled:
		return true
	default:
		return false
	}
}

type GetAuditRequest struct {
	FundID uuid.UUID
	Type   string
	Date   time.Time
}

type donationStore interface {
	GetRecurringDonationsForFund(ctx context.Context, arg donations.GetRecurringDonationsForFundRequest) ([]donations.Donation, error)
	GetPaymentsForDonation(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error)
	GetActiveFunds(ctx context.Context, freq string) ([]donations.Fund, error)
	SetDonationToInactive(ctx context.Context, arg donations.DeactivateDonation) (*donations.Donation, error)
	GetDonationPaymentsByDonationID(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error)
	GetFundByID(ctx context.Context, uuid uuid.UUID) (*donations.Fund, error)
	GetOneTimeDonationsForFund(ctx context.Context, arg donations.GetOneTimeDonationsForFundRequest) ([]donations.Donation, error)
	UpdatePaymentPaypalFee(ctx context.Context, arg donations.UpdatePaymentPaypalFee) (*donations.DonationPayment, error)
	InsertDonationPayment(ctx context.Context, payment donations.InsertDonationPayment) (*donations.DonationPayment, error)
	GetFundPaymentsForAudit(ctx context.Context, fundID uuid.UUID) ([]AuditPayment, error)
	SetPaymentReconciliation(ctx context.Context, arg donations.SetPaymentReconciliation) error
}

type paymentsProvider interface {
	GetProviderDonationSubscriptionStatus(ctx context.Context, providerSubscriptionID string) (string, error)
	GetTransactionsForDonationSubscription(ctx context.Context, subscriptionID string, start, end time.Time) ([]ProviderTransaction, error)
	GetTransaction(ctx context.Context, id string, start, end time.Time) (*ProviderTransaction, error)
}

// documentManager is kept for the reports the finance service still writes to
// object storage. The payments audit no longer uses it: it read a CSV back from
// S3 and now reads the database, so nothing here fetches or lists a stored
// report.
type documentManager interface {
	Upload(ctx context.Context, body io.Reader, name, contentType string) error
}

// eventRecorder writes the fund activity feed. Record does not return an error:
// it runs after the operation it describes has committed.
type eventRecorder interface {
	Record(ctx context.Context, record fundevents.Record)
}

type FinanceService struct {
	donationStore    donationStore
	paymentsProvider paymentsProvider
	documentManager  documentManager
	events           eventRecorder

	reportPrefixes []string

	logger *slog.Logger
}

func NewFinanceService(donationStore donationStore, paymentsProvider paymentsProvider, documentManager documentManager, events eventRecorder, reportPrefixes []string, logger *slog.Logger) *FinanceService {
	return &FinanceService{
		donationStore:    donationStore,
		paymentsProvider: paymentsProvider,
		documentManager:  documentManager,
		events:           events,
		reportPrefixes:   reportPrefixes,
		logger:           logger,
	}
}

// GetAudit is a fund's payments, as they stand.
//
// It used to fetch a CSV from S3 and parse it by column position -- record[5],
// record[8], record[10] -- which is why that file could never carry a header and
// why every unreconciled row padded itself with five empty strings to keep the
// indices aligned. The payments were in the database the whole time; only what
// the provider said about them was not.
func (s FinanceService) GetAudit(ctx context.Context, req GetAuditRequest) (*Audit, error) {
	fund, err := s.donationStore.GetFundByID(ctx, req.FundID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get fund by id", slog.String("error", err.Error()))

		return nil, err
	}

	payments, err := s.donationStore.GetFundPaymentsForAudit(ctx, req.FundID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get payments for audit", slog.String("error", err.Error()))

		return nil, err
	}

	return &Audit{
		FundID:   fund.ID,
		FundName: fund.Name,
		Date:     time.Now(),
		Payments: payments,
	}, nil
}

func (s FinanceService) RunOneTimeDonationReconciliation(ctx context.Context) error {
	funds, err := s.donationStore.GetActiveFunds(ctx, "once")
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get active funds", slog.String("error", err.Error()))

		return err
	}

	for _, fund := range funds {
		errInner := s.reconcileOneTimeDonationsForFund(ctx, fund.ID)
		if errInner != nil {
			return errInner
		}
	}

	return nil
}

// RunRecurringDonationReconciliation covers every frequency whose donors hold
// subscriptions.
//
// It read only "monthly" until daily funds existed, which left a daily fund's
// donations with no status backstop at all -- the one thing this job is for.
// Driving it from Recurring() rather than a literal means the next frequency
// added is covered by adding it there.
func (s FinanceService) RunRecurringDonationReconciliation(ctx context.Context) error {
	for _, frequency := range donations.PayoutFrequencies {
		// Filtered rather than listed. A one-off fund has no subscription to check
		// and its payments are covered by the one-time pass, and a frequency added
		// to PayoutFrequencies is covered here without anybody remembering to.
		if !frequency.Recurring() {
			continue
		}

		funds, err := s.donationStore.GetActiveFunds(ctx, string(frequency))
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to get active funds",
				slog.String("frequency", string(frequency)),
				slog.String("error", err.Error()),
			)

			return err
		}

		for _, fund := range funds {
			if errInner := s.reconcileRecurringDonationsForFund(ctx, fund.ID); errInner != nil {
				return errInner
			}
		}
	}

	return nil
}

// backfillMissingPayments records payments the provider has and we do not.
//
// This is the half of reconciliation that was missing. The existing pass walks
// the payments we already hold and asks the provider about each, so it can report
// a payment that looks wrong and cannot notice one that never arrived -- which is
// exactly what a webhook lost before the bus was durable leaves behind.
//
// Insertion is idempotent on the provider's payment id, so this is safe to run as
// often as it likes: everything already recorded conflicts and does nothing.
func (s FinanceService) backfillMissingPayments(ctx context.Context, donation donations.Donation, known []donations.DonationPayment) (int, error) {
	// Recovery here is by subscription, so a donation without one has nothing to
	// look up. A one-time donation is the case in point: its payment came from a
	// capture, and asking the provider to list transactions for an empty
	// subscription is a request with no answer.
	if donation.ProviderSubscriptionID == "" {
		return 0, nil
	}

	logger := s.logger.With(slog.String("donation_id", donation.ID.String()))

	// From just before the donation existed. Asking earlier is asking about
	// transactions that cannot exist, and PayPal requires a window either way.
	transactions, err := s.paymentsProvider.GetTransactionsForDonationSubscription(ctx,
		donation.ProviderSubscriptionID, donation.Created.AddDate(0, 0, -1), time.Now())
	if err != nil {
		// Returned as well as logged. One unreadable subscription should not stop
		// the other funds, so the caller decides -- but a run where every listing
		// failed used to report success, which is how this endpoint stayed broken:
		// it had never worked, and nothing said so.
		logger.ErrorContext(ctx, "failed to list provider transactions", slog.String("error", err.Error()))

		return 0, err
	}

	have := make(map[string]bool, len(known))
	for _, payment := range known {
		have[payment.ProviderPaymentID] = true
	}

	var recovered int

	for _, transaction := range transactions {
		if transaction.ProviderPaymentID == "" || have[transaction.ProviderPaymentID] {
			continue
		}

		// Only completed money. A pending or failed transaction is not something
		// the fund can pay out, and recording it would inflate the balance.
		if !strings.EqualFold(transaction.Status, "COMPLETED") {
			continue
		}

		inserted, errInsert := s.donationStore.InsertDonationPayment(ctx, donations.InsertDonationPayment{
			ID:                uuid.New(),
			DonationID:        donation.ID,
			ProviderPaymentID: transaction.ProviderPaymentID,
			AmountCents:       transaction.AmountCents,
			ProviderFeeCents:  transaction.FeeCents,
		})
		if errInsert != nil {
			logger.ErrorContext(ctx, "failed to record a payment found at the provider",
				slog.String("provider_payment_id", transaction.ProviderPaymentID),
				slog.String("error", errInsert.Error()),
			)

			continue
		}

		// Nil means another pass or a late webhook got there first.
		if inserted == nil {
			continue
		}

		recovered++

		logger.InfoContext(ctx, "recorded a payment the provider had and we did not",
			slog.String("provider_payment_id", transaction.ProviderPaymentID),
			slog.Int("amount_cents", int(transaction.AmountCents)),
		)

		amount := transaction.AmountCents

		s.events.Record(ctx, fundevents.Record{
			FundID:          donation.FundID,
			Kind:            fundevents.KindPaymentReceived,
			OccurredAt:      transaction.Date,
			SubjectMemberID: &donation.DonorID,
			AmountCents:     &amount,
			Detail:          "recurring, recovered by reconciliation",
			ReferenceID:     &donation.ID,
			// The payment is unique on this already; the key keeps a second
			// reconciliation run from writing a second activity entry for it.
			DedupeKey: "payment-recovered:" + transaction.ProviderPaymentID,
		})
	}

	return recovered, nil
}

func (s FinanceService) reconcileRecurringDonationsForFund(ctx context.Context, fundID uuid.UUID) error {
	logger := s.logger.With(slog.String("fund_id", fundID.String()), slog.String("date", time.Now().Format("01-02-2006")))

	req := donations.GetRecurringDonationsForFundRequest{
		FundID: fundID,
		Active: true,
	}

	recurringDonations, err := s.donationStore.GetRecurringDonationsForFund(ctx, req)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get recurring donations for fund", slog.String("error", err.Error()))

		return err
	}

	for _, donation := range recurringDonations {
		status, errInner := s.paymentsProvider.GetProviderDonationSubscriptionStatus(ctx, donation.ProviderSubscriptionID)
		if errInner != nil {
			// Leave the donation alone: an unreadable status is not evidence that the
			// subscription ended, and deactivating here would cancel a live donation
			// on nothing more than a transient provider error.
			logger.ErrorContext(ctx, "failed to get donation status from provider",
				slog.String("error", errInner.Error()),
				slog.String("donation_id", donation.ID.String()),
			)
		} else if strings.ToUpper(status) != "ACTIVE" {
			logger.InfoContext(ctx, "donation is inactive at provider", slog.String("donation_id", donation.ID.String()))

			_, errInner = s.donationStore.SetDonationToInactive(ctx, donations.DeactivateDonation{
				ID:     donation.ID,
				Reason: status,
			})
			if errInner != nil {
				logger.ErrorContext(ctx, "failed to set donation to inactive", slog.String("error", errInner.Error()))

				return errInner
			}

			// No actor: reconciliation found the provider had already ended it.
			s.events.Record(ctx, fundevents.Record{
				FundID:          donation.FundID,
				Kind:            fundevents.KindDonationCancelled,
				SubjectMemberID: &donation.DonorID,
				Detail:          "subscription " + strings.ToLower(status) + " at provider, found by reconciliation",
				ReferenceID:     &donation.ID,
			})
		}

		payments, errInner := s.donationStore.GetDonationPaymentsByDonationID(ctx, donation.ID)
		if errInner != nil {
			logger.ErrorContext(ctx, "failed to get donation payments", slog.String("error", errInner.Error()))

			return errInner
		}

		// Before the verification pass below, so a payment recovered here is
		// verified and reported in the same run rather than the next one.
		recovered, errBackfill := s.backfillMissingPayments(ctx, donation, payments)
		if errBackfill != nil {
			return errBackfill
		}

		if recovered > 0 {
			payments, errInner = s.donationStore.GetDonationPaymentsByDonationID(ctx, donation.ID)
			if errInner != nil {
				logger.ErrorContext(ctx, "failed to re-read donation payments after backfill",
					slog.String("error", errInner.Error()))

				return errInner
			}
		}

		if errRecord := s.recordProviderView(ctx, payments, logger); errRecord != nil {
			return errRecord
		}
	}

	return nil
}

// recordProviderView asks the provider about each payment and writes the answer
// beside it.
//
// Replaces a CSV per fund per day in S3. The findings belong with the payment:
// the audit page can then answer for a fund as it stands rather than as some past
// run left it, and an amount that does not match is a row somebody can be shown
// rather than a pair of numbers in a file nobody opened.
//
// Returns an error when any write failed. This is the job's only durable output
// now that the CSV is gone, so swallowing a failure would let the run report
// success while the audit page stayed blank and nothing said why. Every payment
// is still attempted first: one bad row should not cost the rest of the fund.
func (s FinanceService) recordProviderView(ctx context.Context, payments []donations.DonationPayment, logger *slog.Logger) error {
	var failed int

	for _, payment := range payments {
		transaction, err := s.paymentsProvider.GetTransaction(ctx,
			payment.ProviderPaymentID, payment.Created.AddDate(0, 0, -1), time.Now())
		if err != nil {
			// Left unreconciled rather than recorded as missing: we did not manage
			// to ask, which is not the same as the provider not knowing.
			logger.ErrorContext(ctx, "failed to get transaction from provider",
				slog.String("payment_id", payment.ID.String()),
				slog.String("error", err.Error()),
			)

			continue
		}

		record := donations.SetPaymentReconciliation{PaymentID: payment.ID}

		if transaction != nil {
			status := transaction.Status
			amount := transaction.AmountCents
			fee := transaction.FeeCents

			record.ProviderStatus = &status
			record.ProviderAmountCents = &amount

			if fee > 0 {
				record.ProviderFeeCents = &fee
			}

			if !strings.EqualFold(transaction.Status, "COMPLETED") {
				logger.WarnContext(ctx, "payment is not complete at the provider",
					slog.String("payment_id", payment.ID.String()),
					slog.String("status", transaction.Status),
				)
			}

			// The one thing this job finds that nothing else would, so it is a
			// warning rather than an info line nobody reads.
			if transaction.AmountCents != payment.AmountCents {
				logger.WarnContext(ctx, "payment amount does not match the provider",
					slog.String("payment_id", payment.ID.String()),
					slog.Int("ours", int(payment.AmountCents)),
					slog.Int("theirs", int(transaction.AmountCents)),
				)
			}
		}

		// Written either way: reconciled_at is what tells the page "checked, and
		// the provider had nothing" apart from "never checked".
		if err = s.donationStore.SetPaymentReconciliation(ctx, record); err != nil {
			logger.ErrorContext(ctx, "failed to record what the provider said",
				slog.String("payment_id", payment.ID.String()),
				slog.String("error", err.Error()),
			)

			failed++
		}
	}

	if failed > 0 {
		return fmt.Errorf("failed to record reconciliation for %d of %d payments", failed, len(payments))
	}

	return nil
}

func (s FinanceService) reconcileOneTimeDonationsForFund(ctx context.Context, fundID uuid.UUID) error {
	logger := s.logger.With(slog.String("fund_id", fundID.String()), slog.String("date", time.Now().Format("01-02-2006")))

	req := donations.GetOneTimeDonationsForFundRequest{
		FundID: fundID,
		Active: true,
	}

	oneTimeDonations, err := s.donationStore.GetOneTimeDonationsForFund(ctx, req)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get one-time donations for fund", slog.String("error", err.Error()))

		return err
	}

	for _, donation := range oneTimeDonations {
		payments, errInner := s.donationStore.GetDonationPaymentsByDonationID(ctx, donation.ID)
		if errInner != nil {
			logger.ErrorContext(ctx, "failed to get donation payments", slog.String("error", errInner.Error()))

			return errInner
		}

		if errRecord := s.recordProviderView(ctx, payments, logger); errRecord != nil {
			return errRecord
		}
	}

	return nil
}
