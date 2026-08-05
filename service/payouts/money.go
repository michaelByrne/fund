package payouts

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func uuidFromString(s string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(s))
}

// dollarStringToCents converts a provider decimal amount such as "12.34" to cents.
//
// An unparseable amount yields 0 rather than an error: these values come from
// webhook payloads where the fee is incidental, and refusing the whole event over a
// malformed fee would lose the status change that actually matters. Amounts that
// decide how much to pay never travel through here.
func dollarStringToCents(decimal string) int32 {
	decimal = strings.TrimSpace(decimal)
	if decimal == "" {
		return 0
	}

	negative := strings.HasPrefix(decimal, "-")
	decimal = strings.TrimPrefix(decimal, "-")

	parts := strings.Split(decimal, ".")
	if len(parts) > 2 {
		return 0
	}

	whole, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}

	fraction := 0
	if len(parts) == 2 {
		frac := parts[1]

		switch {
		case len(frac) > 2:
			frac = frac[:2]
		case len(frac) == 1:
			frac += "0"
		}

		fraction, err = strconv.Atoi(frac)
		if err != nil {
			return 0
		}
	}

	cents := int32(whole*100 + fraction)
	if negative {
		return -cents
	}

	return cents
}
