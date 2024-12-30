package paypal

import (
	"boardfund/service/donations"
	"boardfund/service/finance"
	"context"
	"encoding/json"
	"fmt"
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

func (p Paypal) GetTransactionsForDonationSubscription(ctx context.Context, subscriptionID string) ([]finance.ProviderTransaction, error) {
	transactionsBytes, err := p.client.get(ctx, "/v1/billing/subscriptions/"+subscriptionID+"/transactions")
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

	transactionDate, err := time.Parse("2006-01-02T15:04:05-0700", transactionInfo.TransactionInitiationDate)
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
