package paypal

import (
	"boardfund/service/donations"
	"context"
	"encoding/json"
	"fmt"
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
		ProductID: p.productID,
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
				Description:    fund.Description,
				SoftDescriptor: fund.Name,
				ReferenceID:    "donation",
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

func centsToDecimalString(cents int32) string {
	x := float64(cents)
	x = x / 100
	return fmt.Sprintf("%.2f", x)
}
