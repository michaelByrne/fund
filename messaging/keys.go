package messaging

// Paypal webhook event types, used as subjects on this bus.
const (
	PaymentCompleted          = "PAYMENT.SALE.COMPLETED"
	SubscriptionExpired       = "BILLING.SUBSCRIPTION.EXPIRED"
	SubscriptionSuspended     = "BILLING.SUBSCRIPTION.SUSPENDED"
	SubscriptionCancelled     = "BILLING.SUBSCRIPTION.CANCELLED"
	SubscriptionPaymentFailed = "BILLING.SUBSCRIPTION.PAYMENT.FAILED"
)

// Paypal payout events.
//
// These are an optimisation only. A dropped event must never strand a payout, so
// the payout reconciler polls the provider and is authoritative over anything
// learned here.
const (
	PayoutsBatchSuccess    = "PAYMENT.PAYOUTSBATCH.SUCCESS"
	PayoutsBatchDenied     = "PAYMENT.PAYOUTSBATCH.DENIED"
	PayoutsBatchProcessing = "PAYMENT.PAYOUTSBATCH.PROCESSING"
	PayoutsItemSucceeded   = "PAYMENT.PAYOUTS-ITEM.SUCCEEDED"
	PayoutsItemFailed      = "PAYMENT.PAYOUTS-ITEM.FAILED"
	PayoutsItemBlocked     = "PAYMENT.PAYOUTS-ITEM.BLOCKED"
	PayoutsItemCanceled    = "PAYMENT.PAYOUTS-ITEM.CANCELED"
	PayoutsItemDenied      = "PAYMENT.PAYOUTS-ITEM.DENIED"
	PayoutsItemHeld        = "PAYMENT.PAYOUTS-ITEM.HELD"
	PayoutsItemRefunded    = "PAYMENT.PAYOUTS-ITEM.REFUNDED"
	PayoutsItemReturned    = "PAYMENT.PAYOUTS-ITEM.RETURNED"
	PayoutsItemUnclaimed   = "PAYMENT.PAYOUTS-ITEM.UNCLAIMED"
)
