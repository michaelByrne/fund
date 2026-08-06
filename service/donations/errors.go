package donations

import "errors"

// ErrPaymentAlreadyRecorded means the provider payment is already on record.
//
// A sentinel rather than a plain error because it is a success from the caller's
// point of view: the money is accounted for, and the request that arrived second
// has nothing left to do. Reported as a failure it would turn a double-submitted
// form into a 500 and invite the donor to try again.
var ErrPaymentAlreadyRecorded = errors.New("provider payment already recorded")

// ErrOrderNotComplete means the provider does not agree that the order was paid.
var ErrOrderNotComplete = errors.New("provider order is not complete")

// ErrOrderFundMismatch means the order was created for a different fund than the
// one the request claims. The reference id is set by us when the order is
// created, so a mismatch is not something an honest client can produce.
var ErrOrderFundMismatch = errors.New("provider order belongs to a different fund")

// ErrSubscriptionNotActive means the provider does not consider the subscription
// live, so there is nothing to record.
var ErrSubscriptionNotActive = errors.New("provider subscription is not active")

// ErrSubscriptionPlanMismatch means the subscription pays into a different plan
// than the one being claimed, or that plan belongs to a different fund.
//
// The plan is created by us, per fund, so a mismatch is not something an honest
// client can produce.
var ErrSubscriptionPlanMismatch = errors.New("provider subscription belongs to a different plan or fund")
