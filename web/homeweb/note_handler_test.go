package homeweb

import (
	"context"
	"encoding/gob"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"boardfund/pg"
	"boardfund/service/donations"
	donationsstore "boardfund/service/donations/store"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"
	"boardfund/service/members"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type stubDocumentStorage struct{}

func (stubDocumentStorage) CreateFundBucket(context.Context, string, uuid.UUID) error { return nil }

// stubBucket is the fund image store. Nothing in these tests uploads a picture;
// the service just needs one that is not nil.
type stubBucket struct{}

func (stubBucket) PutFundImage(context.Context, string, string, []byte) error { return nil }

func (stubBucket) GetFundImage(context.Context, string) (io.ReadCloser, error) { return nil, nil }

func (stubBucket) DeleteFundImage(context.Context, string) error { return nil }

// noteRig is the real route behind the real session middleware, so the member
// reaches the handler the way it does in production rather than being injected
// past the code under test.
func noteRig(t *testing.T) (func(fundID, form string) *httptest.ResponseRecorder, *pgxpool.Pool, uuid.UUID) {
	t.Helper()

	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// scs gob-encodes what it stores. Without this it cannot commit the session,
	// answers 500 over a handler that worked, and every assertion below that only
	// looks at the body passes against the failure path instead of the one it names.
	gob.Register(members.Member{})

	sessions := scs.New()

	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

	handlers := NewFundHandlers(
		donations.NewDonationService(
			donationsstore.NewDonationStore(pool), stubDocumentStorage{}, stubBucket{}, nil,
			events, nil, logger,
		),
		events,
		nil,
		nil,
		sessions,
		func(next http.HandlerFunc) http.HandlerFunc { return next },
		logger, "client",
	)

	memberID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO member (id, bco_name, email) VALUES ($1, $2, $3)`,
		memberID, uuid.NewString(), uuid.NewString()+"@test.test")
	require.NoError(t, err)

	router := http.NewServeMux()
	router.HandleFunc("POST /fund/{fundId}/note", func(w http.ResponseWriter, r *http.Request) {
		sessions.Put(r.Context(), "member", members.Member{ID: memberID})

		handlers.saveFundNote(w, r)
	})

	return func(fundID, form string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/fund/"+fundID+"/note", strings.NewReader(form))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		recorder := httptest.NewRecorder()
		sessions.LoadAndSave(router).ServeHTTP(recorder, request)

		return recorder
	}, pool, memberID
}

// seedGift is money given and not refunded, which is what earns the right to
// leave a note. Without it every refusal below stops at the eligibility check and
// the cases that are meant to be about the note never reach it.
func seedGift(t *testing.T, pool *pgxpool.Pool, fundID string, memberID uuid.UUID) {
	t.Helper()

	donationID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id)
		 VALUES ($1, false, $2, $3, $4)`,
		donationID, memberID, uuid.NewString(), fundID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(),
		`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, refunded_cents)
		 VALUES ($1, $2, $3, 5000, 0)`,
		uuid.New(), donationID, uuid.NewString())
	require.NoError(t, err)
}

// seedFund is a fund that really is there, so a refusal that should come from the
// note itself is not quietly coming from the fund lookup instead.
func seedFund(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	fundID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, next_payment)
		 VALUES ($1, $2, 'd', $3, 'paypal', 'once', now())`,
		fundID, uuid.NewString(), fundID.String(),
	)
	require.NoError(t, err)

	return fundID.String()
}

// Every failure in saveFundNote is swapped into the form or the notes section by
// htmx, and a whole layout document put inside a form is not something a browser
// can make sense of -- the section it lands in is the one that breaks, and the
// note the donor typed goes with it. So no path here may answer with a document.
func TestNoteFailuresAnswerWithAFragment(t *testing.T) {
	post, pool, memberID := noteRig(t)

	// A real fund for the refusals that are about the note. Against a fund id that
	// is not there they would all stop at the lookup, and three of these cases
	// would be one case wearing different names.
	fundID := seedFund(t, pool)
	seedGift(t, pool, fundID, memberID)

	// A fund this member has given nothing to, so the eligibility refusal is
	// reachable on its own rather than shadowing the two below it.
	strangerFund := seedFund(t, pool)

	// The message matters as much as the shape. Every one of these is recoverable
	// by the donor, and the generic apology tells them to retype a note the server
	// was never going to take.
	cases := []struct {
		name    string
		fundID  string
		form    string
		message string
	}{
		// Nobody has given to a fund that is not there, so this is refused as not a
		// donor rather than needing a lookup of its own.
		{"a fund that does not exist", uuid.NewString(), "body=hello", "only donors to this fund can leave a note"},
		{"not a fund id at all", "not-a-uuid", "body=hello", "that is not a fund"},
		{"a note with nothing in it", fundID, "body=", "your note needs something in it"},
		{"a note past the limit", fundID, "body=" + strings.Repeat("x", donations.MaxNoteLength+1), donations.ErrNoteTooLong.Error()},
		// Nothing was ever given to this fund, so the note is refused on its merits
		// rather than on its contents -- the path a stranger actually takes.
		{"somebody who has not given", strangerFund, "body=hello", "only donors to this fund can leave a note"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			recorder := post(c.fundID, c.form)

			require.GreaterOrEqual(t, recorder.Code, 400, "these are all refusals")

			html := recorder.Body.String()

			// A fragment, checked by what a document has and a fragment does not.
			for _, marker := range []string{"<html", "<head", "<body", "<nav"} {
				if strings.Contains(html, marker) {
					t.Errorf("answered with a document (%s), which htmx will swap inside the form", marker)
				}
			}

			if !strings.Contains(html, "fund-note-form-") {
				t.Error("a refusal should redraw the form, so the donor can fix it and try again")
			}

			if !strings.Contains(html, c.message) {
				t.Errorf("the donor should be told %q, got:\n%s", c.message, html)
			}
		})
	}
}

// A refusal that empties the box makes the donor retype it, and the longer the
// note the worse that is.
func TestARefusedNoteKeepsWhatWasTyped(t *testing.T) {
	typed := "this fund covered my rent"

	post, _, _ := noteRig(t)

	html := post(uuid.NewString(), "body="+url.QueryEscape(typed)+"&anonymous=true").Body.String()

	if !strings.Contains(html, typed) {
		t.Error("the redrawn form should still hold the note the donor wrote")
	}

	if !strings.Contains(html, "checked") {
		t.Error("and should still remember they asked to post it anonymously")
	}
}

// A page can hold several editors -- my donations draws one per donation -- so a
// reply has to come back wearing the id of the one that asked. Wearing any other,
// the element htmx just replaced is gone and the next edit has nothing to swap.
func TestTheReplyKeepsTheEditorThatAsked(t *testing.T) {
	post, pool, memberID := noteRig(t)

	fundID := seedFund(t, pool)
	seedGift(t, pool, fundID, memberID)

	for _, c := range []struct {
		name   string
		form   string
		status int
	}{
		{"a note that saves", "editor=fund-note-form-mine&body=" + url.QueryEscape("thank you"), 200},
		// The refusals matter more: this is the path htmx has to be able to swap
		// twice, once to show the error and again when the donor fixes it.
		{"a note that is refused", "editor=fund-note-form-mine&body=", 400},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := post(fundID, c.form)
			require.Equal(t, c.status, rec.Code)

			html := rec.Body.String()

			if !strings.Contains(html, `id="fund-note-form-mine"`) {
				t.Errorf("the reply should wear the id of the editor that asked, got:\n%s", html)
			}
		})
	}
}

// Nothing sent, so nothing to echo. A hand-made request should still get an
// editor that works rather than one with no id at all.
func TestAnUnnamedEditorFallsBackToTheFund(t *testing.T) {
	post, pool, memberID := noteRig(t)

	fundID := seedFund(t, pool)
	seedGift(t, pool, fundID, memberID)

	rec := post(fundID, "body="+url.QueryEscape("thank you"))
	require.Equal(t, 200, rec.Code, "this note should have saved")

	html := rec.Body.String()

	if !strings.Contains(html, `id="fund-note-form-`+fundID+`"`) {
		t.Errorf("expected the fund-keyed fallback id, got:\n%s", html)
	}
}
