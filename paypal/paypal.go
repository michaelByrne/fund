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

func (p Paypal) CreateProduct(ctx context.Context, name, description string) (string, error) {
	payload := CreateProduct{
		Name:        name,
		Description: description,
		Type:        "SERVICE",
		Category:    "CHARITY",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	responseBytes, err := p.client.postWithResponse(ctx, "/v1/catalogs/products", payloadBytes)
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
	payload := CreatePlan{
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

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	responseBytes, err := p.client.postWithResponse(ctx, "/v1/billing/plans", payloadBytes)
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

func centsToDecimalString(cents int32) string {
	x := float64(cents)
	x = x / 100
	return fmt.Sprintf("%.2f", x)
}
