package enrollments_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"boardfund/pg"
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
		fundevents.NewService(fundeventstore.NewEventStore(pool), logger),
		logger,
	)

	// A fund whose next payment is a long way off is the case that used to push
	// eligibility furthest out.
	fundID := seedFund(t, ctx, pool, time.Now().Add(90*24*time.Hour))
	memberID := seedMember(t, ctx, pool)

	enrollment, err := svc.CreateEnrollment(ctx, enrollments.CreateEnrollment{
		MemberID:    memberID,
		FundID:      fundID,
		PaypalEmail: "payee@paypal.test",
	})
	require.NoError(t, err)

	// Deliberately not compared against time.Now() at sub-second precision. The
	// column is written by the database's clock and read by the application's,
	// and those differ -- the container this test runs against sits about 90ms
	// ahead of the host, which is enough to make "now" look like the future.
	assert.WithinDuration(t, time.Now(), enrollment.FirstPayoutDate, time.Minute,
		"a new enrollee should be dated at enrollment, not months out")

	assert.True(t, enrollment.Payable(time.Now().Add(time.Minute)),
		"a new enrollee with a payout address belongs in the next batch")

	// This is the assertion that matters: the eligibility query decides who is
	// actually included, and it compares the column against the database's own
	// clock, so it is immune to the skew above.
	var eligible bool
	err = pool.QueryRow(ctx,
		`SELECT first_payout_date <= now() FROM fund_enrollment WHERE id = $1`,
		enrollment.ID,
	).Scan(&eligible)
	require.NoError(t, err)
	assert.True(t, eligible, "the payout query must include a member who just enrolled")
}
