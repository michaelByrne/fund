package enrollments_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"boardfund/pg"
	"boardfund/service/donations"
	donationstore "boardfund/service/donations/store"
	"boardfund/service/enrollments"
	enrollmentstore "boardfund/service/enrollments/store"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedFund(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nextPayment time.Time) uuid.UUID {
	t.Helper()

	fundID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, next_payment)
		 VALUES ($1, 'Test Fund', 'd', $2, 'paypal', 'monthly', $3)`,
		fundID, fundID.String(), nextPayment,
	)
	require.NoError(t, err)

	return fundID
}

func seedMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	memberID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO member (id, email, bco_name, active) VALUES ($1, $2, $3, true)`,
		memberID, memberID.String()+"@test.org", "m-"+memberID.String()[:8],
	)
	require.NoError(t, err)

	return memberID
}

// Enrolling used to date someone at fund.next_payment + 1 month, so a fund whose
// next payment was months out left a new enrollee ineligible for months -- and
// nothing advances next_payment, so the wait applied unevenly depending only on
// when the fund happened to be created.
func TestEnrolleesAreEligibleImmediately(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := enrollments.NewEnrollmentsService(
		enrollmentstore.NewEnrollmentStore(pool),
		donationstore.NewDonationStore(pool),
		fundevents.NewService(fundeventstore.NewEventStore(pool), logger),
		logger,
	)

	// An anchor over a year in the past: the schedule has to roll forward to a
	// real upcoming date rather than handing back something stale.
	anchor := time.Now().AddDate(-1, 0, -3)
	fundID := seedFund(t, ctx, pool, anchor)
	memberID := seedMember(t, ctx, pool)

	enrollment, err := svc.CreateEnrollment(ctx, enrollments.CreateEnrollment{
		MemberID:    memberID,
		FundID:      fundID,
		PaypalEmail: "payee@paypal.test",
	})
	require.NoError(t, err)

	// The fund's next scheduled payout, not the moment of enrolment: the column
	// names a payout date and should hold one. For an anchor three days before
	// today's date a year ago, that is three days from now.
	fund := donations.Fund{PayoutFrequency: donations.PayoutFrequencyMonthly, NextPayment: anchor}
	assert.WithinDuration(t, fund.NextPaymentAfter(time.Now()), enrollment.FirstPayoutDate, time.Minute,
		"first payout should be the fund's next scheduled payout")

	// No waiting period means at most one period away, never the two the old
	// next_payment + 1 month rule could produce.
	assert.True(t, enrollment.FirstPayoutDate.Before(time.Now().AddDate(0, 1, 1)),
		"a new enrollee should not be waiting more than one period")
}

// Taking somebody off a fund is an administrative act, and the feed has to say
// who took it. Recorded with no actor it rendered as "automatic", which credits a
// sweep for a decision a person made -- the one distinction ByProvider exists to
// preserve, and the only admin action on that feed that lost it.
func TestRemovingSomebodyFromAFundNamesWhoDidIt(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
	svc := enrollments.NewEnrollmentsService(
		enrollmentstore.NewEnrollmentStore(pool),
		donationstore.NewDonationStore(pool),
		events,
		logger,
	)

	fundID := seedFund(t, ctx, pool, time.Now())
	memberID := seedMember(t, ctx, pool)
	adminID := seedMember(t, ctx, pool)

	enrollment, err := svc.CreateEnrollment(ctx, enrollments.CreateEnrollment{
		MemberID:    memberID,
		FundID:      fundID,
		PaypalEmail: "payee@paypal.test",
	})
	require.NoError(t, err)

	_, err = svc.DeactivateEnrollment(ctx, enrollment.ID, &adminID)
	require.NoError(t, err)

	// Soft, so the row and anything already paid against it survive. The domain
	// type drops the column, so this asks the row.
	var active bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT active FROM fund_enrollment WHERE id = $1`, enrollment.ID).Scan(&active))
	assert.False(t, active, "the row should stay, deactivated")

	recorded, err := events.GetFundEvents(ctx, fundID, fundevents.DefaultLimit)
	require.NoError(t, err)
	require.NotEmpty(t, recorded)

	cancellation := recorded[0]

	require.Equal(t, fundevents.KindEnrollmentCancelled, cancellation.Kind)
	assert.False(t, cancellation.ByProvider(), "an admin did this, not a sweep")
	require.NotNil(t, cancellation.ActorMemberID)
	assert.Equal(t, adminID, *cancellation.ActorMemberID, "who removed them")
	require.NotNil(t, cancellation.SubjectMemberID)
	assert.Equal(t, memberID, *cancellation.SubjectMemberID, "who was removed")

	// It is about one identifiable member, so it stays off the page donors read.
	public, err := events.GetPublicFundEvents(ctx, fundID, fundevents.DefaultLimit)
	require.NoError(t, err)

	for _, event := range public {
		assert.NotEqual(t, fundevents.KindEnrollmentCancelled, event.Kind)
	}
}
