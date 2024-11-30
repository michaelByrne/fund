package paypal

type ErrPaypal struct {
	Name    string       `json:"name"`
	Message string       `json:"message"`
	DebugID string       `json:"debug_id"`
	Details []FieldError `json:"details"`
}

type FieldError struct {
	Field       string `json:"field"`
	Value       string `json:"value"`
	Location    string `json:"location"`
	Issue       string `json:"issue"`
	Description string `json:"description"`
}

func (e ErrPaypal) Error() string {
	return e.Message
}

type CreateProduct struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Category    string `json:"category"`
}

type CreateProductResponse struct {
	ID string `json:"id"`
}

type CreatePlan struct {
	Name               string             `json:"name"`
	ProductID          string             `json:"product_id"`
	BillingCycles      []BillingCycles    `json:"billing_cycles"`
	PaymentPreferences PaymentPreferences `json:"payment_preferences"`
}

type BillingCycles struct {
	TenureType    string        `json:"tenure_type"`
	Sequence      int32         `json:"sequence"`
	Frequency     Frequency     `json:"frequency"`
	PricingScheme PricingScheme `json:"pricing_scheme"`
	TotalCycles   int32         `json:"total_cycles"`
}

type Frequency struct {
	IntervalUnit  string `json:"interval_unit"`
	IntervalCount int32  `json:"interval_count"`
}

type PaymentPreferences struct {
	SetupFee SetupFee `json:"setup_fee"`
}

type SetupFee struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type PricingScheme struct {
	FixedPrice FixedPrice `json:"fixed_price"`
}

type FixedPrice struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type CreatePlanResponse struct {
	ID string `json:"id"`
}
