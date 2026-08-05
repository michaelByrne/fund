package jwtauth

import "testing"

// Cognito delivers cognito:groups as a JSON array, which lands in the claim map as
// []any. Anything else -- a missing claim, a single string, a wrong-typed value --
// must read as "not a member" rather than panicking on a type assertion.
func TestHasGroup(t *testing.T) {
	cases := []struct {
		name   string
		claims map[string]any
		want   bool
	}{
		{
			name:   "member of the admin group",
			claims: map[string]any{GroupsClaim: []any{"bco-admin-group"}},
			want:   true,
		},
		{
			name:   "admin group alongside others",
			claims: map[string]any{GroupsClaim: []any{"other", "bco-admin-group"}},
			want:   true,
		},
		{
			name:   "groups present but not admin",
			claims: map[string]any{GroupsClaim: []any{"donors"}},
			want:   false,
		},
		{
			name:   "empty group list",
			claims: map[string]any{GroupsClaim: []any{}},
			want:   false,
		},
		{
			name:   "no groups claim at all",
			claims: map[string]any{"custom:member_id": "x"},
			want:   false,
		},
		{
			name:   "claim is nil",
			claims: map[string]any{GroupsClaim: nil},
			want:   false,
		},
		{
			name:   "claim is a bare string, not an array",
			claims: map[string]any{GroupsClaim: "bco-admin-group"},
			want:   false,
		},
		{
			name:   "array holds non-strings",
			claims: map[string]any{GroupsClaim: []any{1, true}},
			want:   false,
		},
		{
			name:   "empty claims",
			claims: map[string]any{},
			want:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasGroup(c.claims, AdminGroup); got != c.want {
				t.Errorf("HasGroup(%v, %q) = %v, want %v", c.claims, AdminGroup, got, c.want)
			}
		})
	}
}
