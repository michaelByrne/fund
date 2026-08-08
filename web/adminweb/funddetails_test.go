package adminweb

import (
	"bytes"
	"context"
	"encoding/gob"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"boardfund/pg"
	"boardfund/service/donations"
	donationsstore "boardfund/service/donations/store"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"
	"boardfund/service/members"
	"boardfund/service/mocks"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type stubDocs struct{}

func (stubDocs) CreateFundBucket(context.Context, string, uuid.UUID) error { return nil }

type stubImages struct{}

func (stubImages) PutFundImage(context.Context, string, string, []byte) error  { return nil }
func (stubImages) GetFundImage(context.Context, string) (io.ReadCloser, error) { return nil, nil }
func (stubImages) DeleteFundImage(context.Context, string) error               { return nil }

// detailsRig drives the real route through the real session middleware against a
// real database, because what is being tested is which columns move.
func detailsRig(t *testing.T) (func(fundID, form string) *httptest.ResponseRecorder, *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	// Without this scs cannot commit the session and answers 500 over a handler
	// that worked, which makes every status assertion below meaningless.
	gob.Register(members.Member{})

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	sessions := scs.New()

	handlers := &AdminHandlers{
		sessionManager: sessions,
		donationService: donations.NewDonationService(
			donationsstore.NewDonationStore(pool), stubDocs{}, stubImages{},
			&mocks.PaymentsProviderMock{},
			fundevents.NewService(fundeventstore.NewEventStore(pool), logger), nil, logger,
		),
	}

	router := http.NewServeMux()
	router.HandleFunc("POST /admin/fund/details/{id}", func(w http.ResponseWriter, r *http.Request) {
		sessions.Put(r.Context(), "member", members.Member{ID: uuid.New()})

		handlers.saveFundDetails(w, r)
	})

	return func(fundID, form string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/admin/fund/details/"+fundID,
			strings.NewReader(form))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		recorder := httptest.NewRecorder()
		sessions.LoadAndSave(router).ServeHTTP(recorder, request)

		return recorder
	}, pool
}

func seedFundRow(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	fundID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, next_payment, goal_cents)
		 VALUES ($1, 'human fund', 'the original', $2, 'paypal', 'monthly', now(), 50000)`,
		fundID, fundID.String())
	require.NoError(t, err)

	return fundID
}

type storedFund struct {
	Name        string
	Description string
	Frequency   string
	GoalCents   int32
	Active      bool
	Expires     *time.Time
}

func readFund(t *testing.T, pool *pgxpool.Pool, fundID uuid.UUID) storedFund {
	t.Helper()

	var fund storedFund
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT name, description, payout_frequency, goal_cents, active, expires
		 FROM fund WHERE id = $1`, fundID).
		Scan(&fund.Name, &fund.Description, &fund.Frequency, &fund.GoalCents, &fund.Active, &fund.Expires))

	return fund
}

// UpdateFund writes whatever it is handed, so the handler carries the untouched
// fields over from the stored fund. Anything it forgets is blanked.
func TestFundDetailsChangesOnlyWhatItShould(t *testing.T) {
	post, pool := detailsRig(t)

	t.Run("saves the editable fields", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		recorder := post(fundID.String(),
			"description="+url.QueryEscape("a new description")+"&goal=250.50&date=2027-03-04")
		require.Equal(t, http.StatusOK, recorder.Code)

		fund := readFund(t, pool, fundID)
		require.Equal(t, "a new description", fund.Description)
		require.Equal(t, int32(25050), fund.GoalCents)
		require.NotNil(t, fund.Expires)
		require.Equal(t, "2027-03-04", fund.Expires.UTC().Format("2006-01-02"))
	})

	// The name is the PayPal product name and the descriptor a donor sees on their
	// card statement. Changing our copy would leave the two disagreeing, and a
	// donor reading a statement is looking at PayPal's.
	t.Run("ignores a name it is sent", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		post(fundID.String(), "description=d&name="+url.QueryEscape("something else"))

		require.Equal(t, "human fund", readFund(t, pool, fundID).Name)
	})

	// Frequency decides which plan a subscription is bound to. Changing it under
	// donors who are already subscribed is worse than not offering it.
	t.Run("ignores a frequency it is sent", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		post(fundID.String(), "description=d&frequency=daily")

		require.Equal(t, "monthly", readFund(t, pool, fundID).Frequency)
	})

	// Nothing on this form is about whether a fund is open, and a save that closed
	// one would stop it taking donations without saying so.
	t.Run("leaves the fund open", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		post(fundID.String(), "description=d&active=false")

		require.True(t, readFund(t, pool, fundID).Active)
	})

	t.Run("clearing the goal is allowed", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		post(fundID.String(), "description=d&goal=")

		require.Zero(t, readFund(t, pool, fundID).GoalCents, "a fund may stop having a target")
	})

	t.Run("clearing the end date is allowed", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		post(fundID.String(), "description=d&date=2027-01-01")
		require.NotNil(t, readFund(t, pool, fundID).Expires)

		post(fundID.String(), "description=d&date=")
		require.Nil(t, readFund(t, pool, fundID).Expires, "a fund may stop ending")
	})

	// A date box gives a day and time.Parse reads it as midnight UTC, so anything
	// rendering it in local time hands back the day before west of Greenwich.
	// Opening the form and saving it again would then walk the date backwards, one
	// day per save, without anybody touching the field.
	t.Run("saving twice does not move the end date", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		post(fundID.String(), "description=d&date=2027-03-04")

		first := readFund(t, pool, fundID)
		require.NotNil(t, first.Expires)

		// What the form would put in the box, fed back in as a browser would.
		redrawn := expiresValue(first.Expires)
		require.Equal(t, "2027-03-04", redrawn, "the box should show the day that was saved")

		post(fundID.String(), "description=d&date="+redrawn)

		second := readFund(t, pool, fundID)
		require.Equal(t, first.Expires.UTC(), second.Expires.UTC(), "the date moved on its own")
	})

	t.Run("a bad amount changes nothing", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		recorder := post(fundID.String(), "description="+url.QueryEscape("changed")+"&goal=lots")
		require.Equal(t, http.StatusBadRequest, recorder.Code)

		fund := readFund(t, pool, fundID)
		require.Equal(t, "the original", fund.Description, "a refusal should not half-save")
		require.Equal(t, int32(50000), fund.GoalCents)

		// And the card comes back holding what is stored, not what was typed.
		require.Contains(t, recorder.Body.String(), "the original")
	})

	t.Run("a bad date changes nothing", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		recorder := post(fundID.String(), "description=changed&date=the-first")
		require.Equal(t, http.StatusBadRequest, recorder.Code)

		require.Equal(t, "the original", readFund(t, pool, fundID).Description)
	})
}

// Creating a fund also creates the PayPal product and plan. A picture that will
// not upload cannot undo those, so the fund has to survive and the admin has to
// be told -- otherwise they make a second fund to replace one that already exists.
func TestCreatingAFundSurvivesABadPicture(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	gob.Register(members.Member{})

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	sessions := scs.New()

	provider := &mocks.PaymentsProviderMock{
		CreateFundFunc: func(context.Context, string, string) (string, error) {
			return uuid.NewString(), nil
		},
		CreatePlanFunc: func(context.Context, donations.CreatePlan) (string, error) {
			return uuid.NewString(), nil
		},
	}

	handlers := &AdminHandlers{
		sessionManager: sessions,
		donationService: donations.NewDonationService(
			donationsstore.NewDonationStore(pool), stubDocs{}, stubImages{}, provider,
			fundevents.NewService(fundeventstore.NewEventStore(pool), logger), nil, logger,
		),
	}

	router := http.NewServeMux()
	router.HandleFunc("POST /admin/fund", func(w http.ResponseWriter, r *http.Request) {
		sessions.Put(r.Context(), "member", members.Member{ID: uuid.New()})

		handlers.createFund(w, r)
	})

	name := "fund " + uuid.NewString()

	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	require.NoError(t, writer.WriteField("name", name))
	require.NoError(t, writer.WriteField("description", "what it is for"))
	require.NoError(t, writer.WriteField("frequency", "monthly"))

	part, err := writer.CreateFormFile("image", "not-really.jpg")
	require.NoError(t, err)

	_, err = part.Write([]byte("this is text, not a picture"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/admin/fund", bytes.NewReader(out.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())

	recorder := httptest.NewRecorder()
	sessions.LoadAndSave(router).ServeHTTP(recorder, request)

	var stored int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM fund WHERE name = $1`, name).Scan(&stored))
	require.Equal(t, 1, stored, "the fund must exist even though its picture did not")

	body := recorder.Body.String()
	require.Contains(t, body, name+" was created", "the admin has to be told the fund exists")
	require.Contains(t, body, "not a jpeg", "and what was wrong with the picture")
}

// A closed fund is finished: its payouts are settled and its donations have
// stopped. Reopening is not something this does, and an editable end date on one
// is how it would happen by accident.
func TestAClosedFundCannotBeChanged(t *testing.T) {
	post, pool := detailsRig(t)

	// closedFund is one past its end date. Expired rather than deactivated, because
	// that is the state nobody pressed a button to reach.
	closedFund := func(t *testing.T) uuid.UUID {
		t.Helper()

		fundID := uuid.New()
		_, err := pool.Exec(context.Background(),
			`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, next_payment, goal_cents, expires)
			 VALUES ($1, 'human fund', 'the original', $2, 'paypal', 'monthly', now(), 50000, now() - INTERVAL '1 day')`,
			fundID, fundID.String())
		require.NoError(t, err)

		return fundID
	}

	t.Run("the server refuses, whatever the page drew", func(t *testing.T) {
		fundID := closedFund(t)

		recorder := post(fundID.String(), "description=changed&date=2099-01-01")
		require.Equal(t, http.StatusBadRequest, recorder.Code)

		fund := readFund(t, pool, fundID)
		require.Equal(t, "the original", fund.Description)
	})

	// The one that matters. A closed fund whose end date can be pushed into the
	// future is a fund taking donations again with its payouts already settled.
	t.Run("the end date cannot be pushed out", func(t *testing.T) {
		fundID := closedFund(t)

		before := readFund(t, pool, fundID)

		post(fundID.String(), "description=d&date=2099-12-31")

		after := readFund(t, pool, fundID)
		require.Equal(t, before.Expires.UTC(), after.Expires.UTC(), "the fund was reopened")
	})

	t.Run("a deactivated fund is refused the same way", func(t *testing.T) {
		fundID := seedFundRow(t, pool)
		_, err := pool.Exec(context.Background(), `UPDATE fund SET active = false WHERE id = $1`, fundID)
		require.NoError(t, err)

		post(fundID.String(), "description=changed")

		require.Equal(t, "the original", readFund(t, pool, fundID).Description)
	})

	t.Run("an open fund is still editable", func(t *testing.T) {
		fundID := seedFundRow(t, pool)

		recorder := post(fundID.String(), "description=changed")
		require.Equal(t, http.StatusOK, recorder.Code)

		require.Equal(t, "changed", readFund(t, pool, fundID).Description)
	})

	// Hiding the form is a courtesy. The card is what says so.
	t.Run("the card offers nothing to press", func(t *testing.T) {
		past := time.Now().Add(-24 * time.Hour)
		fund := donations.Fund{
			ID: uuid.New(), Name: "human fund", Description: "d",
			Active: true, Expires: &past,
		}

		html := renderAdmin(t, FundDetails(fund, nil, "", ""))

		require.NotContains(t, html, "<form")
		require.NotContains(t, html, "type=\"date\"")
		require.NotContains(t, html, "save")
		require.NotContains(t, html, "remove")
		require.Contains(t, html, "this fund is closed")

		// It still says what the fund was.
		require.Contains(t, html, "human fund")
	})
}
