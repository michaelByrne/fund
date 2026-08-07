package paypal

import (
	"boardfund/service/donations"
	"boardfund/service/finance"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Paypal struct {
	client    *Client
	productID string
}

func NewPaypal(client *Client, productID string) *Paypal {
	return &Paypal{
		productID: productID,
		client:    client,
	}
}

func (p Paypal) CancelSubscriptions(ctx context.Context, ids []string) ([]string, error) {
	var cancelledIDs []string
	for _, id := range ids {
		request := CancelSubscriptionRequest{
			Reason: "customer cancelled",
		}

		err := p.client.post(ctx, "/v1/billing/subscriptions/"+id+"/cancel", request)
		if err != nil {
			return cancelledIDs, err
		}

		cancelledIDs = append(cancelledIDs, id)
	}

	return cancelledIDs, nil
}

func (p Paypal) CreateFund(ctx context.Context, name, description string) (string, error) {
	payload := CreateProduct{
		Name:        name,
		Description: description,
		Type:        "SERVICE",
		Category:    "CHARITY",
	}

	responseBytes, err := p.client.postWithResponse(ctx, "/v1/catalogs/products", payload)
	if err != nil {
		return "", err
	}

	var response CreateProductResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		return "", err
	}

	return response.ID, nil
}

func (p Paypal) CreatePlan(ctx context.Context, plan donations.CreatePlan) (string, error) {
	payload := CreatePlanRequest{
		Name:      plan.Name,
		ProductID: plan.ProviderFundID,
		BillingCycles: []BillingCycles{
			{
				TenureType:  "REGULAR",
				Sequence:    1,
				TotalCycles: 0,
				Frequency: Frequency{
					IntervalUnit:  string(plan.IntervalUnit),
					IntervalCount: 1,
				},
				PricingScheme: PricingScheme{
					FixedPrice: FixedPrice{
						CurrencyCode: "USD",
						Value:        centsToDecimalString(plan.AmountCents),
					},
				},
			},
		},
		PaymentPreferences: PaymentPreferences{
			SetupFee: SetupFee{
				CurrencyCode: "USD",
				Value:        "0.0",
			},
		},
	}

	responseBytes, err := p.client.postWithResponse(ctx, "/v1/billing/plans", payload)
	if err != nil {
		return "", err
	}

	var response CreatePlanResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		return "", err
	}

	return response.ID, nil
}

func (p Paypal) ActivatePlan(ctx context.Context, planID string) error {
	return p.client.post(ctx, "/v1/billing/plans/"+planID+"/activate", nil)
}

func (p Paypal) DeactivatePlan(ctx context.Context, planID string) error {
	return p.client.post(ctx, "/v1/billing/plans/"+planID+"/deactivate", nil)
}

func (p Paypal) InitiateDonation(ctx context.Context, fund donations.Fund, amountCents int32) (string, error) {
	orderRequest := CreateOrderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []OrderPurchaseUnits{
			{
				Amount: Amount{
					CurrencyCode: "USD",
					Value:        centsToDecimalString(amountCents),
				},
				Description:    "donation",
				SoftDescriptor: fund.Name,
				ReferenceID:    fund.ID.String(),
			},
		},
	}

	orderResponseBytes, err := p.client.postWithResponse(ctx, "/v2/checkout/orders", orderRequest)
	if err != nil {
		return "", err
	}

	var orderResponse CreateOrderResponse
	err = json.Unmarshal(orderResponseBytes, &orderResponse)
	if err != nil {
		return "", err
	}

	return orderResponse.ID, nil
}

// GetOrder reads an order back from PayPal so a one-time donation can be
// recorded from what the provider says happened rather than from what the
// browser reports.
//
// Everything that matters is already ours: the reference id is the fund we set
// when the order was created, and the capture is the money PayPal actually took.
// The client only needs to name the order.
func (p Paypal) GetOrder(ctx context.Context, orderID string) (*donations.ProviderOrder, error) {
	orderBytes, err := p.client.get(ctx, "/v2/checkout/orders/"+orderID)
	if err != nil {
		return nil, err
	}

	var order PaymentCaptureResponse
	if err = json.Unmarshal(orderBytes, &order); err != nil {
		return nil, err
	}

	result := donations.ProviderOrder{
		Status: order.Status,
	}

	if len(order.PurchaseUnits) == 0 {
		return &result, nil
	}

	unit := order.PurchaseUnits[0]
	result.FundReferenceID = unit.ReferenceID

	// The captured amount, not the requested one. An order can be authorised for
	// one figure and captured for another, and only what was captured is money the
	// fund actually holds.
	for _, capture := range unit.Payments.Captures {
		if capture.Status != "COMPLETED" {
			continue
		}

		result.ProviderPaymentID = capture.ID
		result.AmountCents = decimalDollarStringToCents(capture.Amount.Value)

		break
	}

	return &result, nil
}

// GetSubscription reads a subscription back so a recurring donation can be
// recorded from what the provider says it is, rather than from what the browser
// claims.
//
// plan_id is the part that matters. It is ours, created for one fund, so it is
// what ties a subscription to the fund whose balance its payments will join.
func (p Paypal) GetSubscription(ctx context.Context, subscriptionID string) (*donations.ProviderSubscription, error) {
	subscriptionBytes, err := p.client.get(ctx, "/v1/billing/subscriptions/"+subscriptionID)
	if err != nil {
		return nil, err
	}

	var subscription Subscription
	if err = json.Unmarshal(subscriptionBytes, &subscription); err != nil {
		return nil, err
	}

	return &donations.ProviderSubscription{
		Status:         subscription.Status,
		ProviderPlanID: subscription.PlanID,
		AmountCents:    decimalDollarStringToCents(subscription.BillingInfo.LastPayment.Amount.Value),
	}, nil
}

func (p Paypal) GetProviderDonationSubscriptionStatus(ctx context.Context, providerSubscriptionID string) (string, error) {
	subscriptionBytes, err := p.client.get(ctx, "/v1/billing/subscriptions/"+providerSubscriptionID)
	if err != nil {
		return "", err
	}

	var subscription Subscription
	err = json.Unmarshal(subscriptionBytes, &subscription)
	if err != nil {
		return "", err
	}

	return subscription.Status, nil
}

// GetTransactionsForDonationSubscription lists what a subscription has paid
// between two instants.
//
// start_time and end_time are required by PayPal, and this asked for neither, so
// every call returned MISSING_REQUIRED_PARAMETER and the reconciliation that
// depends on it recovered nothing. The window is the caller's because only the
// caller knows how far back is worth asking: a donation created last week has no
// transactions before it existed.
func (p Paypal) GetTransactionsForDonationSubscription(ctx context.Context, subscriptionID string, start, end time.Time) ([]finance.ProviderTransaction, error) {
	path := "/v1/billing/subscriptions/" + subscriptionID + "/transactions" +
		"?start_time=" + url.QueryEscape(start.UTC().Format(time.RFC3339)) +
		"&end_time=" + url.QueryEscape(end.UTC().Format(time.RFC3339))

	transactionsBytes, err := p.client.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var transactions SubscriptionTransactions
	err = json.Unmarshal(transactionsBytes, &transactions)
	if err != nil {
		return nil, err
	}

	var providerTransactions []finance.ProviderTransaction
	for _, transaction := range transactions.Transactions {
		providerTransactions = append(providerTransactions, finance.ProviderTransaction{
			ProviderPaymentID: transaction.ID,
			Status:            transaction.Status,
			AmountCents:       decimalDollarStringToCents(transaction.AmountWithBreakdown.GrossAmount.Value),
			Date:              transaction.Time,
		})
	}

	return providerTransactions, nil
}

func (p Paypal) GetTransaction(ctx context.Context, id string, start, end time.Time) (*finance.ProviderTransaction, error) {
	path := "/v1/reporting/transactions"
	path += "?start_date=" + start.Format(time.RFC3339)
	path += "&end_date=" + end.Format(time.RFC3339)
	path += "&transaction_id=" + id

	transactionBytes, err := p.client.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var transaction Transaction
	err = json.Unmarshal(transactionBytes, &transaction)
	if err != nil {
		return nil, err
	}

	if transaction.TotalItems == 0 {
		return nil, nil
	}

	var transactionInfo TransactionInfo
	if len(transaction.TransactionDetails) > 0 {
		transactionInfo = transaction.TransactionDetails[0].TransactionInfo
	}

	var status string
	if transactionInfo.TransactionStatus == "S" {
		status = "COMPLETED"
	} else {
		status = "OTHER"
	}

	transactionDate, err := parseProviderTime(transactionInfo.TransactionInitiationDate)
	if err != nil {
		return nil, err
	}

	return &finance.ProviderTransaction{
		ProviderPaymentID: transactionInfo.TransactionID,
		Date:              transactionDate,
		Status:            status,
		AmountCents:       decimalDollarStringToCents(transactionInfo.TransactionAmount.Value),
	}, nil
}

// providerTimeLayouts are the shapes PayPal timestamps arrive in.
//
// The reporting API returns UTC as a trailing Z, and this parsed with a layout
// demanding a numeric offset -- so every transaction it returned failed to parse
// and the whole lookup errored. RFC3339 covers Z and +hh:mm; the third is the
// offset without its colon, which some PayPal responses use and RFC3339 rejects.
var providerTimeLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05",
}

func parseProviderTime(value string) (time.Time, error) {
	var err error

	for _, layout := range providerTimeLayouts {
		var parsed time.Time

		parsed, err = time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unreadable provider timestamp %q: %w", value, err)
}

func centsToDecimalString(cents int32) string {
	x := float64(cents)
	x = x / 100
	return fmt.Sprintf("%.2f", x)
}

func decimalDollarStringToCents(decimal string) int32 {
	decimal = strings.TrimSpace(decimal)

	parts := strings.Split(decimal, ".")
	if len(parts) > 2 {
		// Invalid decimal format
		return 0
	}

	whole, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}

	fraction := 0
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			parts[1] = parts[1][:2]
		} else if len(parts[1]) == 1 {
			parts[1] += "0"
		}

		fraction, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0
		}
	}

	cents := int32(whole*100 + fraction)
	return cents
}
