package donations

// SubscriptionEndedForTest drives the BILLING.SUBSCRIPTION.* handler from an
// external test, so the provider echo can be replayed against the same database
// the local cancellation just wrote to.
func (h *Handlers) SubscriptionEndedForTest(data []byte) error {
	return h.subscriptionEnded(data)
}
