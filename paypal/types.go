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
	Links         []Link                 `json:"links"`
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

type Link struct {
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
	Links                     []Link                    `json:"links"`
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
	//PaymentSource OrderPaymentSource   `json:"payment_source"`
}

type ExperienceContext struct {
	PaymentMethodPreference string `json:"payment_method_preference"`
	BrandName               string `json:"brand_name"`
	Locale                  string `json:"locale"`
	LandingPage             string `json:"landing_page"`
	ShippingPreference      string `json:"shipping_preference"`
	UserAction              string `json:"user_action"`
	ReturnURL               string `json:"return_url"`
	CancelURL               string `json:"cancel_url"`
}
type OrderPaypal struct {
	ExperienceContext ExperienceContext `json:"experience_context"`
}
type OrderPaymentSource struct {
	Paypal OrderPaypal `json:"paypal"`
}

type OrderPurchaseUnits struct {
	ReferenceID    string `json:"reference_id"`
	CustomID       string `json:"custom_id"`
	Amount         Amount `json:"amount"`
	Description    string `json:"description"`
	SoftDescriptor string `json:"soft_descriptor"`
}

type CreateOrderResponse struct {
	ID    string `json:"id"`
	Links []Link `json:"links"`
}

type Updates []struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

type CancelSubscriptionRequest struct {
	Reason string `json:"reason"`
}

type Subscription struct {
	ID               string         `json:"id"`
	PlanID           string         `json:"plan_id"`
	StartTime        time.Time      `json:"start_time"`
	Quantity         string         `json:"quantity"`
	ShippingAmount   ShippingAmount `json:"shipping_amount"`
	Subscriber       Subscriber     `json:"subscriber"`
	BillingInfo      BillingInfo    `json:"billing_info"`
	CreateTime       time.Time      `json:"create_time"`
	UpdateTime       time.Time      `json:"update_time"`
	Links            []Links        `json:"links"`
	Status           string         `json:"status"`
	StatusUpdateTime time.Time      `json:"status_update_time"`
}
type ShippingAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}
type ShippingAddress struct {
	Address Address `json:"address"`
}

type Subscriber struct {
	ShippingAddress ShippingAddress `json:"shipping_address"`
	Name            Name            `json:"name"`
	EmailAddress    string          `json:"email_address"`
	PayerID         string          `json:"payer_id"`
}
type OutstandingBalance struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}
type CycleExecutions struct {
	TenureType      string `json:"tenure_type"`
	Sequence        int    `json:"sequence"`
	CyclesCompleted int    `json:"cycles_completed"`
	CyclesRemaining int    `json:"cycles_remaining"`
	TotalCycles     int    `json:"total_cycles"`
}

type LastPayment struct {
	Amount Amount    `json:"amount"`
	Time   time.Time `json:"time"`
}
type BillingInfo struct {
	OutstandingBalance  OutstandingBalance `json:"outstanding_balance"`
	CycleExecutions     []CycleExecutions  `json:"cycle_executions"`
	LastPayment         LastPayment        `json:"last_payment"`
	NextBillingTime     time.Time          `json:"next_billing_time"`
	FailedPaymentsCount int                `json:"failed_payments_count"`
}
type Links struct {
	Href   string `json:"href"`
	Rel    string `json:"rel"`
	Method string `json:"method"`
}

type SubscriptionTransactions struct {
	Transactions []Transactions `json:"transactions"`
	Links        []Links        `json:"links"`
}
type PayerName struct {
	GivenName string `json:"given_name"`
	Surname   string `json:"surname"`
}

type FeeAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type AmountWithBreakdown struct {
	GrossAmount GrossAmount `json:"gross_amount"`
	FeeAmount   FeeAmount   `json:"fee_amount"`
	NetAmount   NetAmount   `json:"net_amount"`
}
type Transactions struct {
	ID                  string              `json:"id"`
	Status              string              `json:"status"`
	PayerEmail          string              `json:"payer_email"`
	PayerName           PayerName           `json:"payer_name"`
	AmountWithBreakdown AmountWithBreakdown `json:"amount_with_breakdown"`
	Time                time.Time           `json:"time"`
}

type Transaction struct {
	TransactionDetails    []TransactionDetails `json:"transaction_details"`
	AccountNumber         string               `json:"account_number"`
	LastRefreshedDatetime string               `json:"last_refreshed_datetime"`
	Page                  int                  `json:"page"`
	TotalItems            int                  `json:"total_items"`
	TotalPages            int                  `json:"total_pages"`
	Links                 []Links              `json:"links"`
}
type TransactionAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type TransactionInfo struct {
	PaypalAccountID           string            `json:"paypal_account_id"`
	TransactionID             string            `json:"transaction_id"`
	TransactionEventCode      string            `json:"transaction_event_code"`
	TransactionInitiationDate string            `json:"transaction_initiation_date"`
	TransactionUpdatedDate    string            `json:"transaction_updated_date"`
	TransactionAmount         TransactionAmount `json:"transaction_amount"`
	FeeAmount                 FeeAmount         `json:"fee_amount"`
	TransactionStatus         string            `json:"transaction_status"`
	ProtectionEligibility     string            `json:"protection_eligibility"`
}

type PayerInfo struct {
	AccountID     string    `json:"account_id"`
	EmailAddress  string    `json:"email_address"`
	AddressStatus string    `json:"address_status"`
	PayerStatus   string    `json:"payer_status"`
	PayerName     PayerName `json:"payer_name"`
	CountryCode   string    `json:"country_code"`
}

type ShippingInfo struct {
	Name    string  `json:"name"`
	Method  string  `json:"method"`
	Address Address `json:"address"`
}
type ItemUnitPrice struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}
type ItemAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}
type TaxAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}
type TaxAmounts struct {
	TaxAmount TaxAmount `json:"tax_amount"`
}
type BasicShippingAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}
type TotalItemAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}
type ItemDetails struct {
	ItemCode            string              `json:"item_code"`
	ItemName            string              `json:"item_name"`
	ItemQuantity        string              `json:"item_quantity"`
	ItemUnitPrice       ItemUnitPrice       `json:"item_unit_price"`
	ItemAmount          ItemAmount          `json:"item_amount"`
	TaxAmounts          []TaxAmounts        `json:"tax_amounts"`
	BasicShippingAmount BasicShippingAmount `json:"basic_shipping_amount"`
	TotalItemAmount     TotalItemAmount     `json:"total_item_amount"`
}
type CartInfo struct {
	ItemDetails []ItemDetails `json:"item_details"`
}
type StoreInfo struct {
}
type AuctionInfo struct {
	AuctionSite        string `json:"auction_site"`
	AuctionItemSite    string `json:"auction_item_site"`
	AuctionBuyerID     string `json:"auction_buyer_id"`
	AuctionClosingDate string `json:"auction_closing_date"`
}
type IncentiveInfo struct {
}
type TransactionDetails struct {
	TransactionInfo TransactionInfo `json:"transaction_info"`
}
