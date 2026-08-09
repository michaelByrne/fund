package donations

import (
	"boardfund/service/fundevents"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"strings"
	"time"
)

// eventRecorder writes the fund activity feed. Record does not return an error
// on purpose: it runs after the operation it describes has committed, so there
// is nothing a caller could usefully do about a failure.
type eventRecorder interface {
	Record(ctx context.Context, record fundevents.Record)
}

type DonationService struct {
	donationStore    donationStore
	documentStorage  documentStorage
	fundImages       fundImageStorage
	paymentsProvider PaymentsProvider
	events           eventRecorder

	reportBuckets []string

	logger *slog.Logger
}

func NewDonationService(donationStore donationStore, documentStorage documentStorage, fundImages fundImageStorage, provider PaymentsProvider, events eventRecorder, reportBuckets []string, logger *slog.Logger) *DonationService {
	return &DonationService{
		donationStore:    donationStore,
		documentStorage:  documentStorage,
		fundImages:       fundImages,
		paymentsProvider: provider,
		events:           events,
		logger:           logger,
		reportBuckets:    reportBuckets,
	}
}

func (s DonationService) GetTotalDonatedByFund(ctx context.Context, id uuid.UUID) (int64, error) {
	total, err := s.donationStore.GetTotalDonatedByFundID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get total donated by fund id", slog.String("error", err.Error()))

		return 0, err
	}

	return total, nil
}

// ListActiveFunds is the public front page.
//
// It asked for "once" and "monthly" by name, so a fund created with any other
// frequency simply did not appear -- open, collecting, and invisible to the
// donors it needed. Iterating the canonical list means a frequency added to
// PayoutFrequencies shows up here without anyone remembering to come back.
func (s DonationService) ListActiveFunds(ctx context.Context) ([]Fund, error) {
	var funds []Fund

	for _, frequency := range PayoutFrequencies {
		found, err := s.donationStore.GetActiveFunds(ctx, string(frequency))
		if err != nil {
			s.logger.Error("failed to get active funds",
				slog.String("frequency", string(frequency)),
				slog.String("error", err.Error()),
			)

			return nil, err
		}

		funds = append(funds, found...)
	}

	// The monthly breakdown is deliberately not loaded here. This used to run a
	// query per fund and assign the result to `fund`, which is a copy of the slice
	// element, so every row was fetched and discarded. Nothing consumes Monthly
	// from a list -- the charts are on the fund detail page, which loads its own.
	return funds, nil
}

// ListAllFunds returns every fund, including closed and expired ones.
//
// Separate from ListActiveFunds rather than a flag on it, because the two have
// opposite defaults for a reason: the public page must not offer a donor a fund
// that is closed, and the admin page must not lose one. A shared function with a
// boolean would make it easy to get that backwards at a call site.
func (s DonationService) ListAllFunds(ctx context.Context) ([]Fund, error) {
	funds, err := s.donationStore.GetAllFundsWithStats(ctx)
	if err != nil {
		s.logger.Error("failed to list all funds", slog.String("error", err.Error()))

		return nil, err
	}

	return funds, nil
}

// providerStatusCancelled is what PayPal reports for a subscription that has been
// cancelled, and what its BILLING.SUBSCRIPTION.CANCELLED webhook carries.
const providerStatusCancelled = "CANCELLED"

// subscriptionEndedKey identifies "this subscription reached this state", and is
// shared by everything that can record it.
//
// Cancelling produces two accounts of one event: ours, when we ask the provider
// to stop, and the provider's, when it tells us it has. Both are true and the
// feed wants one of them, so both write under this key and the second is
// discarded by the unique index.
//
// Built in one place because the suppression works only while the strings match
// exactly, and two of the writers are in different files.
func subscriptionEndedKey(providerSubscriptionID, status string) string {
	if providerSubscriptionID == "" {
		// No key rather than a shared empty one, which would collapse every
		// unrelated cancellation into the first.
		return ""
	}

	return "subscription-ended:" + providerSubscriptionID + ":" + status
}

// ErrSubscriptionsNotCancelled means the provider would not cancel every
// subscription, so the fund was left open.
var ErrSubscriptionsNotCancelled = errors.New("could not cancel all subscriptions at the provider")

// DeactivateFund closes a fund: no more donations, and every recurring
// subscription cancelled at the provider.
//
// The provider is called before anything is written, and a failure leaves the
// fund open. The previous order committed first, so a PayPal outage produced a
// fund that was closed locally while donors carried on being charged, with
// nothing to retry it -- the reconciler only pushes the other way, marking
// donations inactive once the provider says they are gone.
//
// Reversing it means the surviving failure mode is subscriptions cancelled and
// the local commit lost, which that same reconciler already repairs.
//
// Partial cancellation is refused rather than half-applied. Closing a fund while
// some donors keep paying into it is worse than not closing it, and "nothing
// happened, try again" is a state an admin can act on.
// actorID is a pointer because a fund can close without anyone closing it. The
// expiry job passes nil, which the event feed renders as automatic rather than
// attributing the closure to a person who was not involved -- and a zero uuid
// here would fail fund_event's foreign key to member anyway.
func (s DonationService) DeactivateFund(ctx context.Context, id uuid.UUID, actorID *uuid.UUID) error {
	recurring, err := s.donationStore.GetRecurringDonationsForFund(ctx, GetRecurringDonationsForFundRequest{
		FundID: id,
		Active: true,
	})
	if err != nil {
		s.logger.Error("failed to read recurring donations before deactivating fund",
			slog.String("error", err.Error()))

		return err
	}

	toCancel := extractProviderSubscriptionIDs(recurring)

	if len(toCancel) > 0 {
		cancelled, errCancel := s.paymentsProvider.CancelSubscriptions(ctx, toCancel)
		if errCancel != nil {
			s.logger.Error("failed to cancel subscriptions, fund left active",
				slog.String("error", errCancel.Error()),
				slog.String("fund_id", id.String()),
			)

			return errCancel
		}

		// Compared by membership, not by count. Equal lengths would also be true
		// of a provider that returned duplicates or a different set of ids, and
		// the question here is whether anything we asked for is still running.
		uncancelled := uncancelledSubscriptions(cancelled, toCancel)
		if len(uncancelled) > 0 {
			s.logger.Error("could not cancel every subscription, fund left active",
				slog.String("fund_id", id.String()),
				slog.String("uncancelled", fmt.Sprintf("%v", uncancelled)),
			)

			return fmt.Errorf("%w: %d of %d remain", ErrSubscriptionsNotCancelled,
				len(uncancelled), len(toCancel))
		}
	}

	deactivated, err := s.donationStore.SetFundAndDonationsToInactive(ctx, id)
	if err != nil {
		s.logger.Error("failed to deactivate fund", slog.String("error", err.Error()))

		return err
	}

	// The closure itself, recorded whether or not anyone was subscribed. Without
	// this a fund with no recurring donors closed silently, which is the case the
	// expiry job produces most often.
	s.events.Record(ctx, fundevents.Record{
		FundID:        id,
		Kind:          fundevents.KindFundClosed,
		ActorMemberID: actorID,
	})

	// Recorded only once everything has actually happened. They used to be
	// written before the provider was called, so a cancellation that failed and
	// was rolled back left the history asserting a donation had been cancelled
	// while it was still running.
	for _, cancelled := range deactivated {
		s.events.Record(ctx, fundevents.Record{
			FundID:          cancelled.FundID,
			Kind:            fundevents.KindDonationCancelled,
			ActorMemberID:   actorID,
			SubjectMemberID: &cancelled.DonorID,
			Detail:          "fund deactivated",
			ReferenceID:     &cancelled.ID,
			DedupeKey:       subscriptionEndedKey(cancelled.ProviderSubscriptionID, providerStatusCancelled),
		})
	}

	return nil
}

// ListDonationsForMember is a donor's own donations.
func (s DonationService) ListDonationsForMember(ctx context.Context, memberID uuid.UUID) ([]MemberDonation, error) {
	donationsForMember, err := s.donationStore.GetDonationsForDonor(ctx, memberID)
	if err != nil {
		s.logger.Error("failed to list donations for member",
			slog.String("member_id", memberID.String()),
			slog.String("error", err.Error()),
		)

		return nil, err
	}

	return donationsForMember, nil
}

// CancelDonationForMember ends a member's own recurring donation.
//
// The provider is called before anything is written, and a failure leaves the
// donation alone. This is the same order DeactivateFund uses and for the same
// reason, which matters more here than anywhere: the donor is watching. Marking
// it cancelled locally and failing at PayPal would tell someone their payments
// had stopped while their card kept being charged, and nothing would correct it
// -- reconciliation only pushes the other way, marking donations inactive once
// the provider says they are gone.
//
// Ownership is checked here rather than trusted from the route, because the id
// comes from the request.
func (s DonationService) CancelDonationForMember(ctx context.Context, donationID, memberID uuid.UUID) error {
	donation, err := s.donationStore.GetDonationByID(ctx, donationID)
	if err != nil {
		// An id that matches nothing gets the same answer as one belonging to
		// somebody else. Distinguishing them would let a member discover which
		// donation ids exist by watching which error comes back.
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDonationNotYours
		}

		s.logger.Error("failed to get donation", slog.String("error", err.Error()))

		return err
	}

	if donation == nil || donation.DonorID != memberID {
		return ErrDonationNotYours
	}

	// Already stopped. Reported as success: the donor asked for it to be off, and
	// it is off. A double-submitted form should not read as a failure.
	if !donation.Active {
		return nil
	}

	if !donation.Recurring || donation.ProviderSubscriptionID == "" {
		return ErrDonationNotCancellable
	}

	cancelled, err := s.paymentsProvider.CancelSubscriptions(ctx, []string{donation.ProviderSubscriptionID})
	if err != nil {
		s.logger.Error("failed to cancel subscription at provider, donation left active",
			slog.String("donation_id", donationID.String()),
			slog.String("error", err.Error()),
		)

		return err
	}

	// Checked by membership rather than count, as elsewhere: a provider that
	// returned a different id of the same length would otherwise pass.
	if len(uncancelledSubscriptions(cancelled, []string{donation.ProviderSubscriptionID})) > 0 {
		s.logger.Error("provider did not cancel the subscription, donation left active",
			slog.String("donation_id", donationID.String()),
		)

		return ErrSubscriptionsNotCancelled
	}

	_, err = s.donationStore.SetDonationToInactive(ctx, DeactivateDonation{
		ID:     donationID,
		Reason: "cancelled by donor",
	})
	if err != nil {
		// The subscription is cancelled at the provider and the row still says
		// active. Reconciliation repairs exactly this, which is why this is the
		// direction the ordering leaves open.
		s.logger.Error("cancelled at provider but failed to record it",
			slog.String("donation_id", donationID.String()),
			slog.String("error", err.Error()),
		)

		return err
	}

	s.events.Record(ctx, fundevents.Record{
		FundID:          donation.FundID,
		Kind:            fundevents.KindDonationCancelled,
		ActorMemberID:   &memberID,
		SubjectMemberID: &memberID,
		Detail:          "cancelled by donor",
		ReferenceID:     &donation.ID,
		DedupeKey:       subscriptionEndedKey(donation.ProviderSubscriptionID, providerStatusCancelled),
	})

	return nil
}

func (s DonationService) DeactivateDonation(ctx context.Context, id, actorID uuid.UUID, reason string) (*Donation, error) {
	donation, err := s.donationStore.SetDonationToInactive(ctx, DeactivateDonation{
		ID:     id,
		Reason: reason,
	})
	if err != nil {
		s.logger.Error("failed to set donation to inactive", slog.String("error", err.Error()))

		return nil, err
	}

	s.events.Record(ctx, fundevents.Record{
		FundID:          donation.FundID,
		Kind:            fundevents.KindDonationCancelled,
		ActorMemberID:   &actorID,
		SubjectMemberID: &donation.DonorID,
		Detail:          reason,
		ReferenceID:     &donation.ID,
	})

	return donation, nil
}

func (s DonationService) ListFunds(ctx context.Context) ([]Fund, error) {
	funds, err := s.donationStore.GetFunds(ctx)
	if err != nil {
		s.logger.Error("failed to list funds", slog.String("error", err.Error()))

		return nil, err
	}

	return funds, nil
}

func (s DonationService) GetFundByID(ctx context.Context, id uuid.UUID) (*Fund, error) {
	fund, err := s.donationStore.GetFundByID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get fund by id", slog.String("error", err.Error()))

		return nil, err
	}

	monthly, err := s.donationStore.GetMonthlyDonationTotalsForFund(ctx, fund.ID)
	if err != nil {
		s.logger.Error("failed to get monthly donation totals for fund", slog.String("error", err.Error()))

		return nil, err
	}

	fund.Stats.Monthly = monthly

	return fund, nil
}

func (s DonationService) CreateDonationPlan(ctx context.Context, plan CreatePlan) (*DonationPlan, error) {
	providerID, err := s.paymentsProvider.CreatePlan(ctx, plan)
	if err != nil {
		s.logger.Error("failed to create plan with provider", slog.String("error", err.Error()))

		return nil, err
	}

	upsertPlan := UpsertDonationPlan{
		ID:             uuid.New(),
		Name:           plan.Name,
		ProviderPlanID: providerID,
		AmountCents:    plan.AmountCents,
		IntervalUnit:   plan.IntervalUnit,
		IntervalCount:  plan.IntervalCount,
		Active:         true,
		FundID:         plan.FundID,
	}

	planOut, err := s.donationStore.UpsertDonationPlan(ctx, upsertPlan)
	if err != nil {
		s.logger.Error("failed to upsert donation plan", slog.String("error", err.Error()))

		return nil, err
	}

	return planOut, nil
}

// CompleteRecurringDonation records a subscription from the provider's account of
// it, not the browser's.
//
// Only the subscription id and the plan being claimed come from the caller. The
// status, the plan actually being paid into and the amount come back from PayPal,
// and the plan has to belong to the fund being credited.
//
// That last check is the one that was costing money. fund_id came from the form
// and was never compared to anything, so a subscription created against one
// fund's plan could be recorded against another -- and every payment on it then
// joined the wrong fund's balance and was paid out to the wrong fund's enrollees.
//
// The one-time path was fixed for the same reason; this is its twin, and it was
// left behind because the payments arrive by signed webhook and the link did not.
func (s DonationService) CompleteRecurringDonation(ctx context.Context, memberID uuid.UUID, completion RecurringCompletion) error {
	subscription, err := s.paymentsProvider.GetSubscription(ctx, completion.ProviderSubscriptionID)
	if err != nil {
		s.logger.Error("failed to read subscription from provider",
			slog.String("provider_subscription_id", completion.ProviderSubscriptionID),
			slog.String("error", err.Error()),
		)

		return err
	}

	if !subscription.Active() {
		s.logger.Error("subscription is not active at the provider",
			slog.String("provider_subscription_id", completion.ProviderSubscriptionID),
			slog.String("status", subscription.Status),
		)

		return ErrSubscriptionNotActive
	}

	if !completion.PlanID.Valid {
		return ErrSubscriptionPlanMismatch
	}

	plan, err := s.donationStore.GetDonationPlanByID(ctx, completion.PlanID.UUID)
	if err != nil {
		s.logger.Error("failed to get donation plan", slog.String("error", err.Error()))

		return err
	}

	// The plan is ours and was created for one fund. Both halves matter: the
	// subscription must pay into the plan being claimed, and that plan must belong
	// to the fund about to be credited.
	if plan == nil || plan.ProviderPlanID != subscription.ProviderPlanID || plan.FundID != completion.FundID {
		s.logger.Error("subscription does not match the plan or fund claimed",
			slog.String("provider_subscription_id", completion.ProviderSubscriptionID),
			slog.String("subscription_plan_id", subscription.ProviderPlanID),
			slog.String("claimed_fund_id", completion.FundID.String()),
		)

		return ErrSubscriptionPlanMismatch
	}

	// The plan's amount, not the form's. It is what the donor actually agreed to
	// pay, and it is what the activity feed reports.
	amountCents := plan.AmountCents

	insertDonation := InsertDonation{
		ID:                     uuid.New(),
		DonorID:                memberID,
		PlanID:                 completion.PlanID,
		FundID:                 completion.FundID,
		ProviderOrderID:        completion.ProviderOrderID,
		ProviderSubscriptionID: completion.ProviderSubscriptionID,
		Recurring:              true,
	}

	_, err = s.donationStore.InsertDonation(ctx, insertDonation)
	if err != nil {
		s.logger.Error("failed to insert donation", slog.String("error", err.Error()))

		return err
	}

	s.events.Record(ctx, fundevents.Record{
		FundID:          completion.FundID,
		Kind:            fundevents.KindDonationStarted,
		ActorMemberID:   &memberID,
		SubjectMemberID: &memberID,
		AmountCents:     &amountCents,
		Detail:          "recurring",
		ReferenceID:     &insertDonation.ID,
	})

	return nil
}

func (s DonationService) InitiateDonation(ctx context.Context, fundID uuid.UUID, amountCents int32) (string, error) {
	fund, err := s.donationStore.GetFundByID(ctx, fundID)
	if err != nil {
		s.logger.Error("failed to get fund by id", slog.String("error", err.Error()))

		return "", err
	}

	orderID, err := s.paymentsProvider.InitiateDonation(ctx, *fund, amountCents)
	if err != nil {
		s.logger.Error("failed to initiate donation with provider", slog.String("error", err.Error()))

		return "", err
	}

	return orderID, nil
}

// CompleteDonation records a one-time donation from the provider's account of the
// order, not the browser's.
//
// Only the order id is taken from the caller. The amount, the payment id and the
// fund all come back from PayPal, so a request cannot claim money that was never
// captured. The recurring path has always worked this way -- it records from a
// signed PAYMENT.SALE.COMPLETED webhook -- and this closes the same gap on the
// one-time path, which trusted the form outright.
func (s DonationService) CompleteDonation(ctx context.Context, memberID uuid.UUID, completion OneTimeCompletion) error {
	order, err := s.paymentsProvider.GetOrder(ctx, completion.ProviderOrderID)
	if err != nil {
		s.logger.Error("failed to read order from provider",
			slog.String("order_id", completion.ProviderOrderID),
			slog.String("error", err.Error()),
		)

		return err
	}

	if order.Status != "COMPLETED" || order.ProviderPaymentID == "" || order.AmountCents <= 0 {
		s.logger.Error("order is not a completed payment",
			slog.String("order_id", completion.ProviderOrderID),
			slog.String("status", order.Status),
			slog.Int("amount_cents", int(order.AmountCents)),
		)

		return ErrOrderNotComplete
	}

	// The reference id was set to the fund when the order was created, so this
	// catches an order for one fund being completed against another.
	if order.FundReferenceID != completion.FundID.String() {
		s.logger.Error("order belongs to a different fund",
			slog.String("order_id", completion.ProviderOrderID),
			slog.String("claimed_fund_id", completion.FundID.String()),
			slog.String("order_fund_id", order.FundReferenceID),
		)

		return ErrOrderFundMismatch
	}

	insertDonation := InsertDonation{
		ID:              uuid.New(),
		DonorID:         memberID,
		FundID:          completion.FundID,
		ProviderOrderID: completion.ProviderOrderID,
	}

	insertPayment := InsertDonationPayment{
		ID:                uuid.New(),
		AmountCents:       order.AmountCents,
		ProviderFeeCents:  order.FeeCents,
		ProviderPaymentID: order.ProviderPaymentID,
		DonationID:        insertDonation.ID,
	}

	_, err = s.donationStore.InsertDonationWithPayment(ctx, insertDonation, insertPayment)
	if err != nil {
		// Already recorded, so the money is accounted for and the second request
		// has nothing to do. Reporting a failure here would show the donor an
		// error for a donation that went through.
		if errors.Is(err, ErrPaymentAlreadyRecorded) {
			s.logger.Info("one-time donation already recorded",
				slog.String("provider_payment_id", order.ProviderPaymentID),
			)

			return nil
		}

		s.logger.Error("failed to create donation with payment", slog.String("error", err.Error()))

		return err
	}

	// The provider's figure, not the caller's. The audit trail is read to answer
	// where the money went, so recording a number nobody verified would put the
	// same lie into the record it was just kept out of the ledger.
	s.events.Record(ctx, fundevents.Record{
		FundID:          completion.FundID,
		Kind:            fundevents.KindDonationStarted,
		ActorMemberID:   &memberID,
		SubjectMemberID: &memberID,
		AmountCents:     &order.AmountCents,
		Detail:          "one-time",
		ReferenceID:     &insertDonation.ID,
	})

	s.events.Record(ctx, fundevents.Record{
		FundID:          completion.FundID,
		Kind:            fundevents.KindPaymentReceived,
		ActorMemberID:   &memberID,
		SubjectMemberID: &memberID,
		AmountCents:     &order.AmountCents,
		ReferenceID:     &insertDonation.ID,
	})

	return nil
}

func (s DonationService) CreateFund(ctx context.Context, createFund Fund) (*Fund, error) {
	providerID, err := s.paymentsProvider.CreateFund(ctx, createFund.Name, createFund.Description)
	if err != nil {
		s.logger.Error("failed to create fund with provider", slog.String("error", err.Error()))

		return nil, err
	}

	insertFund := InsertFund{
		ID:              uuid.New(),
		Name:            createFund.Name,
		Description:     createFund.Description,
		ProviderID:      providerID,
		PayoutFrequency: string(createFund.PayoutFrequency),
		GoalCents:       createFund.GoalCents,
		Expires:         createFund.Expires,
		Active:          true,
		ProviderName:    "paypal",
	}

	fund, err := s.donationStore.InsertFund(ctx, insertFund)
	if err != nil {
		s.logger.Error("failed to insert fund", slog.String("error", err.Error()))

		return nil, err
	}

	err = s.createFundBuckets(ctx, fund.ID)
	if err != nil {
		s.logger.Error("failed to create fund buckets", slog.String("error", err.Error()))

		return nil, err
	}

	return fund, nil
}

// UpdateFund changes a fund that is still open.
//
// A closed one is refused. Reopening is not something this does -- it would mean
// a fund taking donations again with its payouts already settled -- and letting
// an end date be edited on one that has passed is how that would happen by
// accident.
func (s DonationService) UpdateFund(ctx context.Context, updateFund Fund, actorID *uuid.UUID) (*Fund, error) {
	current, err := s.donationStore.GetFundByID(ctx, updateFund.ID)
	if err != nil {
		s.logger.Error("failed to read the fund being updated", slog.String("error", err.Error()))

		return nil, err
	}

	if current.Closed() {
		return nil, ErrFundClosed
	}

	// Computed against what is stored, before the write. Afterwards the previous
	// values are gone: `updated` is overwritten and nothing keeps the old ones.
	changes := describeFundChanges(*current, updateFund)

	update := UpdateFund{
		ID:              updateFund.ID,
		Name:            updateFund.Name,
		Description:     updateFund.Description,
		Active:          updateFund.Active,
		GoalCents:       updateFund.GoalCents,
		PayoutFrequency: string(updateFund.PayoutFrequency),
		Expires:         updateFund.Expires,
	}

	fund, err := s.donationStore.UpdateFund(ctx, update)
	if err != nil {
		s.logger.Error("failed to update fund", slog.String("error", err.Error()))

		return nil, err
	}

	// Only when something moved. The details form posts every field on every
	// save, so a visit that changed nothing would otherwise write a line saying
	// the fund was updated -- and a feed of those teaches a reader that the kind
	// means nothing.
	if changes != "" {
		s.events.Record(ctx, fundevents.Record{
			FundID:        updateFund.ID,
			Kind:          fundevents.KindFundUpdated,
			ActorMemberID: actorID,
			Detail:        changes,
		})
	}

	return fund, nil
}

// describeFundChanges says what an edit actually changed, in the words a reader
// would use.
//
// The old value is included because it is the part that cannot be recovered:
// the new one is in the fund row, and "the goal changed" without saying from
// what is a line that records only that somebody was here.
//
// Name and payout frequency are compared even though the form locks them. A
// field that cannot currently be edited is not a field that will never be
// edited, and this is the place that would silently stop covering it.
func describeFundChanges(before, after Fund) string {
	var changes []string

	if before.Name != after.Name {
		changes = append(changes, fmt.Sprintf("name %q to %q", before.Name, after.Name))
	}

	if before.Description != after.Description {
		// The text itself is not repeated. A description can be paragraphs, and
		// the feed is a list of one-line entries.
		changes = append(changes, "description edited")
	}

	if before.GoalCents != after.GoalCents {
		changes = append(changes, fmt.Sprintf("goal %s to %s",
			goalDescription(before.GoalCents), goalDescription(after.GoalCents)))
	}

	if before.PayoutFrequency != after.PayoutFrequency {
		changes = append(changes, fmt.Sprintf("payouts %s to %s",
			before.PayoutFrequency, after.PayoutFrequency))
	}

	if !sameDay(before.Expires, after.Expires) {
		changes = append(changes, fmt.Sprintf("end date %s to %s",
			expiryDescription(before.Expires), expiryDescription(after.Expires)))
	}

	return strings.Join(changes, ", ")
}

// goalDescription renders cents as dollars, in integer arithmetic.
//
// Widened to int64 and signed separately. The obvious form, "$%d.%02d" of
// cents/100 and cents%100, renders a negative goal as "$-5.-50" -- and a
// negative is reachable, because the goal arrives as a parsed float from a form
// and nothing rejects a minus sign. Negating in int32 would also turn the
// minimum value into itself.
func goalDescription(cents int32) string {
	if cents == 0 {
		return "none"
	}

	value, sign := int64(cents), ""
	if value < 0 {
		value, sign = -value, "-"
	}

	return fmt.Sprintf("%s$%d.%02d", sign, value/100, value%100)
}

// expiryDescription renders the day, in UTC.
//
// UTC because everything else that touches this date already is: sameDay below
// compares UTC days, and the form's date input renders expires.UTC(). Formatting
// in the value's own location would let the recorded line name a different day
// from the one the comparison decided had changed, and from the one the admin
// saw in the field they edited.
func expiryDescription(expires *time.Time) string {
	if expires == nil {
		return "none"
	}

	return expires.UTC().Format("2006-01-02")
}

// sameDay compares the two expiry values as dates.
//
// The form submits a day, which parses to midnight UTC, while the stored value
// came back from Postgres as a timestamptz in whatever location the driver
// chose. Comparing those with == reports a change on every save of an unchanged
// date, which is exactly the empty event this guards against.
func sameDay(before, after *time.Time) bool {
	if before == nil || after == nil {
		return before == after
	}

	beforeYear, beforeMonth, beforeDay := before.UTC().Date()
	afterYear, afterMonth, afterDay := after.UTC().Date()

	return beforeYear == afterYear && beforeMonth == afterMonth && beforeDay == afterDay
}

func (s DonationService) createFundBuckets(ctx context.Context, fundID uuid.UUID) error {
	for _, prefix := range s.reportBuckets {
		err := s.documentStorage.CreateFundBucket(ctx, prefix, fundID)
		if err != nil {
			s.logger.Error("failed to create fund bucket", slog.String("error", err.Error()))
		}
	}

	return nil
}

func extractProviderSubscriptionIDs(donations []Donation) []string {
	var subscriptionIDs []string

	for _, donation := range donations {
		if donation.ProviderSubscriptionID != "" {
			subscriptionIDs = append(subscriptionIDs, donation.ProviderSubscriptionID)
		}
	}

	return subscriptionIDs
}

func uncancelledSubscriptions(cancelled []string, all []string) []string {
	var uncancelled []string

	for _, sub := range all {
		var found bool
		for _, c := range cancelled {
			if sub == c {
				found = true
				break
			}
		}

		if !found {
			uncancelled = append(uncancelled, sub)
		}
	}

	return uncancelled
}

// CloseExpiredFunds deactivates every fund whose end date has passed.
//
// Expiry used to hide a fund from the public listing and nothing else, so a fund
// past its end date kept collecting: recurring subscriptions were never
// cancelled, and donors carried on being charged for something that had already
// finished, with no page left in the UI to notice it on.
//
// This runs the same DeactivateFund a person would, so the provider is called
// before anything is written and partial cancellation is refused. A fund that
// fails to close stays open and is retried on the next run, which is the right
// outcome: leaving it open is visible, whereas closing it locally while donors
// keep paying is not.
//
// Ordering matters against the payout planner. Closing a fund stops new batches
// being planned for it, so the planner must run first on the day a fund expires
// or its final period is never paid. Already-approved batches are unaffected --
// submission does not check whether the fund is still open.
func (s DonationService) CloseExpiredFunds(ctx context.Context) (int, error) {
	expired, err := s.ListExpiredOpenFunds(ctx)
	if err != nil {
		return 0, err
	}

	closed := 0
	for _, fund := range expired {
		// No actor: nobody closed this, the date did. DeactivateFund records the
		// event with a nil actor, which is what makes the feed distinguish it from
		// a treasurer shutting a fund down.
		if errClose := s.DeactivateFund(ctx, fund.ID, nil); errClose != nil {
			s.logger.Error("failed to close expired fund",
				slog.String("error", errClose.Error()),
				slog.String("fund_id", fund.ID.String()),
				slog.String("fund", fund.Name),
			)

			continue
		}

		s.logger.Info("closed expired fund",
			slog.String("fund_id", fund.ID.String()),
			slog.String("fund", fund.Name),
		)

		closed++
	}

	s.logger.Info("expired fund closure complete",
		slog.Int("expired", len(expired)),
		slog.Int("closed", closed),
	)

	return closed, nil
}

// ListExpiredOpenFunds is what CloseExpiredFunds would act on, for the dry run.
// Shared with the closer rather than re-derived, so the preview cannot describe
// a different set than the one that gets closed.
func (s DonationService) ListExpiredOpenFunds(ctx context.Context) ([]Fund, error) {
	expired, err := s.donationStore.GetExpiredActiveFunds(ctx)
	if err != nil {
		s.logger.Error("failed to get expired funds", slog.String("error", err.Error()))

		return nil, err
	}

	return expired, nil
}

// ListClosedFunds returns the public archive of funds that have ended, each with
// what it collected and what it paid out.
func (s DonationService) ListClosedFunds(ctx context.Context) ([]ClosedFund, error) {
	// One query, aggregates included. This drives the front page and the archive
	// only grows, so a stats lookup per closed fund would cost a round-trip per
	// row on every home page load, forever.
	funds, err := s.donationStore.GetClosedFundsWithStats(ctx)
	if err != nil {
		s.logger.Error("failed to list closed funds", slog.String("error", err.Error()))

		return nil, err
	}

	return funds, nil
}

// GetClosedFund is the summary page for one ended fund: what came in, what went
// out, and the month-by-month breakdown a recurring fund is worth seeing.
func (s DonationService) GetClosedFund(ctx context.Context, fundID uuid.UUID) (*ClosedFund, error) {
	fund, err := s.GetFundByID(ctx, fundID)
	if err != nil {
		return nil, err
	}

	stats, err := s.donationStore.GetFundPayoutStats(ctx, fundID)
	if err != nil {
		s.logger.Error("failed to get payout stats for fund",
			slog.String("error", err.Error()),
			slog.String("fund_id", fundID.String()),
		)

		return nil, err
	}

	return &ClosedFund{Fund: *fund, Payouts: stats}, nil
}
