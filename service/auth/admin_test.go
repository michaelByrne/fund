package auth

import (
	"boardfund/jwtauth"
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

func newTestService(f *fakeAuthorizer) AuthService {
	return AuthService{
		authorizer: f,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestGrantAndRevokeAdminUseTheCognitoGroup(t *testing.T) {
	fake := &fakeAuthorizer{groups: map[string][]string{"michael": {"some-other-group"}}}
	svc := newTestService(fake)

	ctx := context.Background()

	if err := svc.GrantAdmin(ctx, "michael"); err != nil {
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

	if err := svc.RevokeAdmin(ctx, "michael"); err != nil {
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
