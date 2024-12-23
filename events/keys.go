package events

// Paypal events
const (
	SubscriptionPaymentCompleted string = "PAYMENT.SALE.COMPLETED"
	SubscriptionExpired                 = "BILLING.SUBSCRIPTION.EXPIRED"
	SubscriptionSuspended               = "BILLING.SUBSCRIPTION.SUSPENDED"
	SubscriptionCancelled               = "BILLING.SUBSCRIPTION.CANCELLED"
	SubscriptionPaymentFailed           = "BILLING.SUBSCRIPTION.PAYMENT.FAILED"
)
