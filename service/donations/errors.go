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

// ErrDonationNotYours means the donation belongs to somebody else.
//
// Reported as not-found to the caller rather than forbidden: a member has no
// business learning whether a donation id exists.
var ErrDonationNotYours = errors.New("donation does not belong to this member")

// ErrDonationNotCancellable means there is nothing to cancel -- a one-off
// donation, or one already ended.
var ErrDonationNotCancellable = errors.New("donation cannot be cancelled")

// ErrFundClosed means the fund has ended -- deactivated, or past its end date --
// and is not something to be changed any more.
//
// Enforced in the service rather than by hiding the controls. A closed fund with
// an editable end date reads as one that can be reopened, and the form being
// absent is a courtesy: the rule has to hold for anything that posts to the
// route, not only for what the page drew.
var ErrFundClosed = errors.New("a closed fund cannot be changed")

// ErrOneTimeFundNeedsEndDate refuses a one-off fund with no end date.
//
// For that frequency the end date is the payout date: InsertFund anchors
// next_payment to it, and that is the only date the fund ever pays on.
//
// Leaving it out did not produce a fund that never paid. The
// before_insert_or_update_fund trigger fills both expires and next_payment one
// month out when a 'once' fund omits its end date, so the fund had a payout date
// -- just not one anybody chose, arriving from a database trigger nothing in the
// application mentions.
//
// That is what this refuses. A date that decides when money moves should be
// picked by the person creating the fund, not defaulted invisibly a layer below
// the code that reads it. Recurring funds are different: their schedule stands
// on its own and an end date is genuinely optional.
var ErrOneTimeFundNeedsEndDate = errors.New("a one-time fund needs an end date, which is when it pays out")
