package paypal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The reporting API states a fee as money leaving, so it arrives negative.
// Passed through unchanged it would add the fee back to a balance that is
// supposed to be having it taken off.
func TestAProviderFeeIsReadAsAPositiveCost(t *testing.T) {
	for _, c := range []struct {
		name  string
		value string
		want  int32
	}{
		{"negative, as the reporting api sends it", "-0.56", 56},
		{"positive, as the orders api sends it", "0.56", 56},
		{"whitespace around it", "  -1.01  ", 101},
		{"no fee", "0.00", 0},
		{"empty", "", 0},
		{"larger than a dollar", "-12.34", 1234},
	} {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, feeCentsFromProvider(c.value))
		})
	}
}

// The fee on a one-time donation comes from the capture's own breakdown. It was
// parsed into this struct and never read, so every one-time donation was recorded
// as though it had cost nothing to collect.
func TestGetOrderReadsTheCaptureFee(t *testing.T) {
	response := PaymentCaptureResponse{
		Status: "COMPLETED",
		PurchaseUnits: []CapturePurchaseUnits{
			{
				ReferenceID: "a-fund",
				Payments: Payments{
					Captures: []Captures{
						{
							ID:     "CAPTURE-1",
							Status: "COMPLETED",
							Amount: Amount{CurrencyCode: "USD", Value: "5.00"},
							SellerReceivableBreakdown: SellerReceivableBreakdown{
								GrossAmount: GrossAmount{CurrencyCode: "USD", Value: "5.00"},
								PaypalFee:   PaypalFee{CurrencyCode: "USD", Value: "0.66"},
								NetAmount:   NetAmount{CurrencyCode: "USD", Value: "4.34"},
							},
						},
					},
				},
			},
		},
	}

	order := orderFromResponse(response)

	require.Equal(t, int32(500), order.AmountCents)
	require.Equal(t, int32(66), order.FeeCents, "the fee the account was charged to collect it")
	require.Equal(t, "CAPTURE-1", order.ProviderPaymentID)
}

// A capture that has not completed is skipped, and PayPal documents the
// breakdown as absent while a transaction is pending -- so nothing should be
// read off one.
func TestAPendingCaptureContributesNothing(t *testing.T) {
	order := orderFromResponse(PaymentCaptureResponse{
		Status: "PENDING",
		PurchaseUnits: []CapturePurchaseUnits{
			{
				ReferenceID: "a-fund",
				Payments: Payments{
					Captures: []Captures{
						{ID: "CAPTURE-1", Status: "PENDING", Amount: Amount{Value: "5.00"}},
					},
				},
			},
		},
	})

	require.Zero(t, order.AmountCents)
	require.Zero(t, order.FeeCents)
	require.Empty(t, order.ProviderPaymentID)
}

// Reconciliation is the only thing that can fill in a fee no webhook carried, and
// it never could: this mapping dropped the fee, so the value it handed the
// reconciler was always zero and the guard that only writes a positive fee never
// fired. A payment could be fully reconciled -- status, amount, timestamp -- and
// still record no fee at all.
func TestReconciliationReadsTheTransactionFee(t *testing.T) {
	transaction := Transaction{
		TotalItems: 1,
		TransactionDetails: []TransactionDetails{
			{
				TransactionInfo: TransactionInfo{
					TransactionID:             "7LL546189S889181P",
					TransactionStatus:         "S",
					TransactionInitiationDate: "2026-08-07T18:29:16Z",
					TransactionAmount:         TransactionAmount{CurrencyCode: "USD", Value: "4.00"},
					// As the reporting API sends it: money leaving, so negative.
					FeeAmount: FeeAmount{CurrencyCode: "USD", Value: "-0.63"},
				},
			},
		},
	}

	result, err := transactionFromResponse(transaction)
	require.NoError(t, err)

	require.Equal(t, int32(400), result.AmountCents)
	require.Equal(t, int32(63), result.FeeCents, "positive, because every caller subtracts it")
	require.Equal(t, "COMPLETED", result.Status)
}
