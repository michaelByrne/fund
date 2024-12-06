package paypal

import "time"

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

type CreatePlanRequest struct {
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

type PaymentCaptureResponse struct {
	ID            string                 `json:"id"`
	Status        string                 `json:"status"`
	PaymentSource PaymentSource          `json:"payment_source"`
	PurchaseUnits []CapturePurchaseUnits `json:"purchase_units"`
	Payer         Payer                  `json:"payer"`
	Links         []Links                `json:"links"`
}

type Name struct {
	GivenName string `json:"given_name"`
	Surname   string `json:"surname"`
}

type PaymentSource struct {
	Paypal Payer `json:"paypal"`
}

type Address struct {
	AddressLine1 string `json:"address_line_1"`
	AddressLine2 string `json:"address_line_2"`
	AdminArea2   string `json:"admin_area_2"`
	AdminArea1   string `json:"admin_area_1"`
	PostalCode   string `json:"postal_code"`
	CountryCode  string `json:"country_code"`
}

type Shipping struct {
	Address Address `json:"address"`
}

type Amount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type SellerProtection struct {
	Status            string   `json:"status"`
	DisputeCategories []string `json:"dispute_categories"`
}

type GrossAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type PaypalFee struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type NetAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type SellerReceivableBreakdown struct {
	GrossAmount GrossAmount `json:"gross_amount"`
	PaypalFee   PaypalFee   `json:"paypal_fee"`
	NetAmount   NetAmount   `json:"net_amount"`
}

type Links struct {
	Href   string `json:"href"`
	Rel    string `json:"rel"`
	Method string `json:"method"`
}

type Captures struct {
	ID                        string                    `json:"id"`
	Status                    string                    `json:"status"`
	Amount                    Amount                    `json:"amount"`
	SellerProtection          SellerProtection          `json:"seller_protection"`
	FinalCapture              bool                      `json:"final_capture"`
	DisbursementMode          string                    `json:"disbursement_mode"`
	SellerReceivableBreakdown SellerReceivableBreakdown `json:"seller_receivable_breakdown"`
	CreateTime                time.Time                 `json:"create_time"`
	UpdateTime                time.Time                 `json:"update_time"`
	Links                     []Links                   `json:"links"`
}

type Payments struct {
	Captures []Captures `json:"captures"`
}

type CapturePurchaseUnits struct {
	ReferenceID string   `json:"reference_id"`
	Shipping    Shipping `json:"shipping"`
	Payments    Payments `json:"payments"`
}

type Payer struct {
	Name         Name   `json:"name"`
	EmailAddress string `json:"email_address"`
	PayerID      string `json:"payer_id"`
}

type CreateOrderRequest struct {
	Intent        string               `json:"intent"`
	PurchaseUnits []OrderPurchaseUnits `json:"purchase_units"`
}

type OrderPurchaseUnits struct {
	ReferenceID    string `json:"reference_id"`
	CustomID       string `json:"custom_id"`
	Amount         Amount `json:"amount"`
	Description    string `json:"description"`
	SoftDescriptor string `json:"soft_descriptor"`
}

type CreateOrderResponse struct {
	ID string `json:"id"`
}

type Updates []struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}
