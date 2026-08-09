package adminevents_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"boardfund/pg"
	"boardfund/service/adminevents"
	admineventstore "boardfund/service/adminevents/store"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Approving an email address is the gate: nothing else decides who can create an
// account. Its subject cannot be a member, because being approved is what lets
// them become one -- which is why admin_event.subject_member_id is nullable.
func TestApprovingAnEmailIsRecordedAgainstTheAddress(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := adminevents.NewService(admineventstore.NewEventStore(pool), logger)

	t.Run("a subject with no account is recorded by label", func(t *testing.T) {
		actor := seedMember(t, ctx, pool, "admin")

		svc.Record(ctx, adminevents.Record{
			Kind:          adminevents.KindEmailApproved,
			ActorMemberID: &actor,
			SubjectLabel:  "newcomer@test.org",
		})

		events, errEvents := svc.GetAdminEvents(ctx, adminevents.DefaultLimit)
		require.NoError(t, errEvents)
		require.NotEmpty(t, events)

		event := events[0]

		assert.Equal(t, adminevents.KindEmailApproved, event.Kind)
		assert.False(t, event.AboutAMember(), "there is no member page to link to")
		assert.Equal(t, "newcomer@test.org", event.Subject())
		assert.NotEmpty(t, event.ActorName, "who opened the door is the point")

		// The audit page joins to member for the subject. An inner join would drop
		// exactly these rows, which is the failure that looks like nothing.
		assert.Nil(t, event.SubjectMemberID)
	})

	// Both, or neither, and the check constraint says the same thing. A row with
	// neither describes nothing; one with both leaves a reader guessing.
	t.Run("a record without exactly one subject is refused before the database", func(t *testing.T) {
		before, errBefore := svc.GetAdminEvents(ctx, adminevents.DefaultLimit)
		require.NoError(t, errBefore)

		member := seedMember(t, ctx, pool, "both")

		for name, bad := range map[string]adminevents.Record{
			"neither": {Kind: adminevents.KindEmailApproved},
			"both": {
				Kind:            adminevents.KindEmailApproved,
				SubjectMemberID: &member,
				SubjectLabel:    "also@test.org",
			},
		} {
			var out bytes.Buffer

			adminevents.NewService(
				admineventstore.NewEventStore(pool),
				slog.New(slog.NewJSONHandler(&out, nil)),
			).Record(ctx, bad)

			// The check constraint would refuse these too, so counting rows cannot
			// tell whether the service did. The message is what distinguishes
			// "declined without asking" from "asked and was told no", and the first
			// is what the guard is for.
			assert.Contains(t, out.String(), "refusing to record an admin event",
				"%s: the service should refuse this itself", name)
			assert.NotContains(t, out.String(), "failed to record admin event",
				"%s: it should not have reached the database", name)
		}

		after, errAfter := svc.GetAdminEvents(ctx, adminevents.DefaultLimit)
		require.NoError(t, errAfter)

		assert.Len(t, after, len(before))
	})
}

// The address goes in the table, where the admins who can already read it in the
// admin section will find it. It stays out of the log, which is a stream with
// different retention and a stated rule of ids only.
func TestTheApprovedAddressDoesNotReachTheLog(t *testing.T) {
	var out bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&out, nil))
	actor := uuid.New()

	adminevents.NewService(stubStore{}, logger).Record(context.Background(), adminevents.Record{
		Kind:          adminevents.KindEmailApproved,
		ActorMemberID: &actor,
		SubjectLabel:  "newcomer@test.org",
	})

	body := out.String()

	if strings.Contains(body, "newcomer@test.org") {
		t.Errorf("the log line carries an email address:\n%s", body)
	}

	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(body)), &record))

	// Still says one happened, and that its subject was not a member -- otherwise
	// a reader would have to guess why the line has no subject at all.
	assert.Equal(t, "email_approved", record["kind"])
	assert.Equal(t, true, record["subject_not_a_member"])
	assert.Equal(t, actor.String(), record["actor_member_id"])
}

// A write that fails has to say the same things about the event as one that
// succeeds. "failed to record admin event, kind=admin_granted" leaves the only
// question worth asking -- whose access went unrecorded -- answerable by
// guessing.
func TestAFailedWriteSaysWhoItWasAbout(t *testing.T) {
	var out bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&out, nil))
	actor, subject := uuid.New(), uuid.New()

	adminevents.NewService(failingStore{}, logger).Record(context.Background(), adminevents.Record{
		Kind:            adminevents.KindAdminGranted,
		ActorMemberID:   &actor,
		SubjectMemberID: &subject,
	})

	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out.String())), &record))

	assert.Equal(t, "ERROR", record["level"])
	assert.Equal(t, subject.String(), record["subject_member_id"])
	assert.Equal(t, actor.String(), record["actor_member_id"])
	assert.NotEmpty(t, record["error"])
}

// And the same on the other shape of subject, without the address.
func TestAFailedWriteAboutANonMemberStillWithholdsTheAddress(t *testing.T) {
	var out bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&out, nil))

	adminevents.NewService(failingStore{}, logger).Record(context.Background(), adminevents.Record{
		Kind:         adminevents.KindEmailApproved,
		SubjectLabel: "newcomer@test.org",
	})

	body := out.String()

	assert.NotContains(t, body, "newcomer@test.org")
	assert.Contains(t, body, `"subject_not_a_member":true`)
}

type stubStore struct{}

func (stubStore) InsertAdminEvent(_ context.Context, arg adminevents.Record) (*adminevents.Event, error) {
	return &adminevents.Event{ID: uuid.New(), Kind: arg.Kind}, nil
}

func (stubStore) GetAdminEvents(context.Context, int32) ([]adminevents.Event, error) {
	return nil, nil
}

type failingStore struct{}

func (failingStore) InsertAdminEvent(context.Context, adminevents.Record) (*adminevents.Event, error) {
	return nil, errors.New("database down")
}

func (failingStore) GetAdminEvents(context.Context, int32) ([]adminevents.Event, error) {
	return nil, nil
}
