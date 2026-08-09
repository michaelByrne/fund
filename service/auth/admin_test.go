package auth

import (
	"boardfund/jwtauth"
	"boardfund/service/adminevents"
	"boardfund/service/members"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

// fakeAuthorizer records group writes and replays a canned group list. Only the
// group methods are exercised here; the rest satisfy the interface.
type fakeAuthorizer struct {
	groups map[string][]string
	err    error

	added   []string
	removed []string
}

func (f *fakeAuthorizer) Authorize(context.Context, string, string) (*AuthResponse, error) {
	return nil, nil
}

func (f *fakeAuthorizer) SetPassword(context.Context, string, string, string) error {
	return nil
}

func (f *fakeAuthorizer) CreateUser(context.Context, string, string, uuid.UUID) (string, error) {
	return "", nil
}

func (f *fakeAuthorizer) AddToGroup(_ context.Context, username, group string) error {
	if f.err != nil {
		return f.err
	}

	f.added = append(f.added, username+":"+group)
	f.groups[username] = append(f.groups[username], group)

	return nil
}

func (f *fakeAuthorizer) RemoveFromGroup(_ context.Context, username, group string) error {
	if f.err != nil {
		return f.err
	}

	f.removed = append(f.removed, username+":"+group)

	var kept []string
	for _, g := range f.groups[username] {
		if g != group {
			kept = append(kept, g)
		}
	}
	f.groups[username] = kept

	return nil
}

func (f *fakeAuthorizer) ListGroups(_ context.Context, username string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.groups[username], nil
}

// recorder collects what the service writes to the audit trail.
type recorder struct {
	records []adminevents.Record
}

func (r *recorder) Record(_ context.Context, record adminevents.Record) {
	r.records = append(r.records, record)
}

func newTestService(f *fakeAuthorizer) AuthService {
	return newAuditedTestService(f, &recorder{})
}

func newAuditedTestService(f *fakeAuthorizer, log *recorder) AuthService {
	return AuthService{
		authorizer:  f,
		adminEvents: log,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestGrantAndRevokeAdminUseTheCognitoGroup(t *testing.T) {
	fake := &fakeAuthorizer{groups: map[string][]string{"michael": {"some-other-group"}}}
	svc := newTestService(fake)

	ctx := context.Background()
	actor, subject := testMember("gofreescout"), testMember("michael")

	if err := svc.GrantAdmin(ctx, actor, subject); err != nil {
		t.Fatalf("grant: %v", err)
	}

	// The group name is the entire authorisation decision. Writing any other
	// group would silently grant nothing.
	if len(fake.added) != 1 || fake.added[0] != "michael:"+jwtauth.AdminGroup {
		t.Fatalf("added %v, want one write of michael:%s", fake.added, jwtauth.AdminGroup)
	}

	isAdmin, err := svc.IsAdmin(ctx, "michael")
	if err != nil {
		t.Fatalf("is admin: %v", err)
	}
	if !isAdmin {
		t.Error("member should be an admin after being granted")
	}

	if err := svc.RevokeAdmin(ctx, actor, subject); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if len(fake.removed) != 1 || fake.removed[0] != "michael:"+jwtauth.AdminGroup {
		t.Fatalf("removed %v, want one write of michael:%s", fake.removed, jwtauth.AdminGroup)
	}

	isAdmin, err = svc.IsAdmin(ctx, "michael")
	if err != nil {
		t.Fatalf("is admin: %v", err)
	}
	if isAdmin {
		t.Error("member should not be an admin after revocation")
	}

	// Revoking admin must not disturb the member's other groups.
	if got := fake.groups["michael"]; len(got) != 1 || got[0] != "some-other-group" {
		t.Errorf("other groups = %v, want [some-other-group]", got)
	}
}

func testMember(name string) members.Member {
	return members.Member{ID: uuid.New(), BCOName: name}
}

func TestAdminChangesAreRecordedWithBothParties(t *testing.T) {
	fake := &fakeAuthorizer{groups: map[string][]string{"michael": nil}}
	log := &recorder{}
	svc := newAuditedTestService(fake, log)

	ctx := context.Background()
	actor, subject := testMember("gofreescout"), testMember("michael")

	if err := svc.GrantAdmin(ctx, actor, subject); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := svc.RevokeAdmin(ctx, actor, subject); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if len(log.records) != 2 {
		t.Fatalf("recorded %d events, want 2", len(log.records))
	}

	granted, revoked := log.records[0], log.records[1]

	if granted.Kind != adminevents.KindAdminGranted {
		t.Errorf("first event is %q, want %q", granted.Kind, adminevents.KindAdminGranted)
	}
	if revoked.Kind != adminevents.KindAdminRevoked {
		t.Errorf("second event is %q, want %q", revoked.Kind, adminevents.KindAdminRevoked)
	}

	// The point of the log is attribution. An event naming only the subject
	// records that somebody's access changed without recording who changed it,
	// which is the state this table was added to end.
	for i, record := range log.records {
		if record.SubjectMemberID == nil || *record.SubjectMemberID != subject.ID {
			t.Errorf("event %d: subject %v, want %v", i, record.SubjectMemberID, subject.ID)
		}
		if record.ActorMemberID == nil {
			t.Fatalf("event %d: no actor recorded", i)
		}
		if *record.ActorMemberID != actor.ID {
			t.Errorf("event %d: actor %v, want %v", i, *record.ActorMemberID, actor.ID)
		}
	}
}

func TestAFailedGroupWriteIsNotRecorded(t *testing.T) {
	fake := &fakeAuthorizer{groups: map[string][]string{}, err: errors.New("cognito down")}
	log := &recorder{}
	svc := newAuditedTestService(fake, log)

	ctx := context.Background()
	actor, subject := testMember("gofreescout"), testMember("michael")

	// Both must fail; recording either would be the audit trail asserting a
	// privilege change that never reached Cognito. A log that invents grants is
	// worse than no log, because it will be believed.
	if err := svc.GrantAdmin(ctx, actor, subject); err == nil {
		t.Fatal("a failed group write should return an error")
	}
	if err := svc.RevokeAdmin(ctx, actor, subject); err == nil {
		t.Fatal("a failed group removal should return an error")
	}

	if len(log.records) != 0 {
		t.Errorf("recorded %v, want nothing for a change that did not happen", log.records)
	}
}

func TestAnUnattributedChangeRecordsNoActor(t *testing.T) {
	fake := &fakeAuthorizer{groups: map[string][]string{"michael": nil}}
	log := &recorder{}

	// A zero-value actor is what a caller with no signed-in member holds. The
	// nil uuid is a real value that would render as a member link to nothing, so
	// it must become "not recorded" rather than travel into the log.
	if err := newAuditedTestService(fake, log).GrantAdmin(
		context.Background(), members.Member{}, testMember("michael"),
	); err != nil {
		t.Fatalf("grant: %v", err)
	}

	if len(log.records) != 1 {
		t.Fatalf("recorded %d events, want 1", len(log.records))
	}
	if log.records[0].ActorMemberID != nil {
		t.Errorf("actor %v, want none", *log.records[0].ActorMemberID)
	}
}

func TestIsAdminIgnoresOtherGroups(t *testing.T) {
	fake := &fakeAuthorizer{groups: map[string][]string{
		"gofreescout": {"bco-admin", "admin", "bco-admin-group-x"},
	}}

	// Names that merely resemble the admin group must not grant admin.
	isAdmin, err := newTestService(fake).IsAdmin(context.Background(), "gofreescout")
	if err != nil {
		t.Fatalf("is admin: %v", err)
	}
	if isAdmin {
		t.Error("only an exact group name should grant admin")
	}
}

func TestIsAdminSurfacesLookupFailure(t *testing.T) {
	fake := &fakeAuthorizer{groups: map[string][]string{}, err: errors.New("cognito down")}

	// A failed lookup must not read as "not an admin": the caller renders an
	// unknown state from the error, and swallowing it would show a wrong answer.
	if _, err := newTestService(fake).IsAdmin(context.Background(), "michael"); err == nil {
		t.Fatal("a failed group lookup should return an error")
	}
}
