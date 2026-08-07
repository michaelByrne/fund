package enrollments

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

// The address is where the fund's money goes, and a malformed one is not refused
// by the provider until the payout has been planned and submitted.
//
// What this cannot do is confirm an account exists. PayPal offers no supported
// way to ask, so a valid-looking address with no account behind it is accepted
// here, sits UNCLAIMED, and is auto-returned after thirty days.
func TestValidatePaypalEmail(t *testing.T) {
	accepted := []string{
		"payee@example.com",
		"first.last+fund@example.co.uk",
		"UPPER@Example.Com",
	}

	for _, address := range accepted {
		got, err := validatePaypalEmail(address)
		if err != nil {
			t.Errorf("validatePaypalEmail(%q) rejected a usable address: %v", address, err)
		}

		if got != address {
			t.Errorf("validatePaypalEmail(%q) = %q, want it unchanged", address, got)
		}
	}

	rejected := []struct {
		name    string
		address string
	}{
		{"empty", ""},
		{"only spaces", "   "},
		{"no at sign", "payee.example.com"},
		{"nothing before the at", "@example.com"},
		{"nothing after the at", "payee@"},
		// net/mail accepts a bare host, and PayPal will not deliver to one.
		{"no dot in the domain", "payee@localhost"},
		// net/mail parses this happily and PayPal would look for a recipient
		// called "Someone <payee@example.com>".
		{"display name attached", "Someone <payee@example.com>"},
		{"two addresses", "one@example.com, two@example.com"},
	}

	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			if _, err := validatePaypalEmail(c.address); err == nil {
				t.Errorf("validatePaypalEmail(%q) was accepted", c.address)
			} else if !errors.Is(err, ErrInvalidPaypalEmail) {
				t.Errorf("error should be recognisable to the handler, got %v", err)
			}
		})
	}
}

// Trimmed rather than rejected: a trailing space from a copy-paste is not the
// admin getting it wrong, and the stored address must not carry it.
func TestValidatePaypalEmailTrimsSurroundingSpace(t *testing.T) {
	got, err := validatePaypalEmail("  payee@example.com  ")
	if err != nil {
		t.Fatalf("a pasted address should be accepted: %v", err)
	}

	if got != "payee@example.com" {
		t.Errorf("got %q, want the address without surrounding space", got)
	}
}

// The check lives in the service rather than the form, so it holds for every
// caller. Validation runs before any store is touched, which is what lets this
// construct the service with none.
func TestCreateEnrollmentRefusesAnUnusableAddress(t *testing.T) {
	svc := NewEnrollmentsService(nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	_, err := svc.CreateEnrollment(context.Background(), CreateEnrollment{
		FundID:      uuid.New(),
		MemberID:    uuid.New(),
		PaypalEmail: "not-an-address",
	})

	if !errors.Is(err, ErrInvalidPaypalEmail) {
		t.Fatalf("err = %v, want ErrInvalidPaypalEmail", err)
	}

	// Reaching a nil store would panic, so returning an error at all proves the
	// enrollment was refused before anything was written.
}
