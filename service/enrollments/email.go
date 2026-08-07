package enrollments

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// ErrInvalidPaypalEmail means the address is not a well-formed email.
//
// It says nothing about whether a PayPal account exists for it. PayPal has no
// supported way to ask that -- an endpoint confirming which addresses have
// accounts would be an enumeration oracle, and the one that used to exist is
// deprecated and restricted. A payout to an address with no account is accepted,
// sits UNCLAIMED, and is auto-returned after thirty days.
//
// So this catches the typo and nothing more, which is worth doing and worth not
// overstating.
var ErrInvalidPaypalEmail = errors.New("that is not a valid email address")

// validatePaypalEmail normalises and checks a payee address.
//
// Deliberately stricter than net/mail alone. ParseAddress accepts a display name
// -- "Someone <a@b.com>" parses happily -- and PayPal would treat the whole
// string as the recipient and fail to find anyone. It also accepts a bare host
// with no dot, which cannot be a real mail domain.
func validatePaypalEmail(address string) (string, error) {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return "", fmt.Errorf("%w: it is empty", ErrInvalidPaypalEmail)
	}

	parsed, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidPaypalEmail, trimmed)
	}

	// Anything but the bare address means a display name came along with it.
	if parsed.Address != trimmed {
		return "", fmt.Errorf("%w: give the address on its own, without a name", ErrInvalidPaypalEmail)
	}

	at := strings.LastIndex(parsed.Address, "@")
	if !strings.Contains(parsed.Address[at+1:], ".") {
		return "", fmt.Errorf("%w: %s has no domain", ErrInvalidPaypalEmail, trimmed)
	}

	return trimmed, nil
}
