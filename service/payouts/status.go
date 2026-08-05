package payouts

import "strings"

// ProviderStatusToStatus maps a payments-provider status onto our own.
//
// Anything unrecognised maps to StatusPending rather than a terminal state. An
// unknown status means we do not know whether the money moved, and guessing
// 'failed' would invite a re-plan that pays the same person twice.
func ProviderStatusToStatus(providerStatus string) Status {
	switch strings.ToUpper(strings.TrimSpace(providerStatus)) {
	case "SUCCESS", "COMPLETED":
		return StatusPaid
	case "FAILED", "DENIED":
		return StatusFailed
	case "PENDING", "PROCESSING", "NEW":
		return StatusPending
	case "UNCLAIMED":
		// Recipient has no account for this address. PayPal auto-returns after 30
		// days, so this is not terminal.
		return StatusUnclaimed
	case "RETURNED", "REVERSED", "REFUNDED":
		return StatusReturned
	case "ONHOLD":
		return StatusOnhold
	case "BLOCKED":
		return StatusBlocked
	case "CANCELED", "CANCELLED":
		return StatusCancelled
	default:
		return StatusPending
	}
}
