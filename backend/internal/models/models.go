package models

import (
	"time"
)

// ============================================
// Ticket Types
// ============================================

type TicketType struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	PriceCents     int               `json:"price_cents"`
	QuantityTotal  int               `json:"quantity_total"`
	QuantitySold   int               `json:"quantity_sold"`
	SaleStart      time.Time         `json:"sale_start"`
	SaleEnd        time.Time         `json:"sale_end"`
	IsActive       bool              `json:"is_active"`
	MaxPerOrder    int               `json:"max_per_order"`
	AllowedDomains []string          `json:"allowed_domains"`
	Categories     []TicketCategory  `json:"categories,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type TicketCategory struct {
	ID                string   `json:"id"`
	TicketTypeID      string   `json:"ticket_type_id"`
	Name              string   `json:"name"`
	QuantityAllocated int      `json:"quantity_allocated"`
	QuantitySold      int      `json:"quantity_sold"`
	AllowedDomains    []string `json:"allowed_domains"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Champ calculé pour le frontend
func (tt TicketType) QuantityRemaining() int {
	return tt.QuantityTotal - tt.QuantitySold
}

func (tt TicketType) PriceEuros() float64 {
	return float64(tt.PriceCents) / 100.0
}

func (tt TicketType) IsOnSale() bool {
	now := time.Now()
	return tt.IsActive && now.After(tt.SaleStart) && now.Before(tt.SaleEnd) && tt.QuantityRemaining() > 0
}

// ============================================
// Orders
// ============================================

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusRefunded  OrderStatus = "refunded"
)

type Order struct {
	ID                   string      `json:"id"`
	OrderNumber          string      `json:"order_number"`
	CustomerEmail        string      `json:"customer_email"`
	CustomerFirstName    string      `json:"customer_first_name"`
	CustomerLastName     string      `json:"customer_last_name"`
	CustomerPhone        string      `json:"customer_phone,omitempty"`
	TotalCents           int         `json:"total_cents"`
	Status               OrderStatus `json:"status"`
	HelloAssoCheckoutID  string      `json:"helloasso_checkout_id,omitempty"`
	HelloAssoPaymentID   string      `json:"helloasso_payment_id,omitempty"`
	HelloAssoCheckoutURL string      `json:"helloasso_checkout_url,omitempty"`
	IPAddress            string      `json:"-"`
	UserAgent            string      `json:"-"`
	CreatedAt            time.Time   `json:"created_at"`
	UpdatedAt            time.Time   `json:"updated_at"`
	PaidAt               *time.Time  `json:"paid_at,omitempty"`
	ConfirmedAt          *time.Time  `json:"confirmed_at,omitempty"`
	Tickets              []Ticket    `json:"tickets,omitempty"`
}

// ============================================
// Tickets
// ============================================

type Ticket struct {
	ID                 string     `json:"id"`
	OrderID            string     `json:"order_id"`
	TicketTypeID       string     `json:"ticket_type_id"`
	QRToken            string     `json:"qr_token"`
	QRCodeData         []byte     `json:"-"`
	IsValidated        bool       `json:"is_validated"`
	ValidatedAt        *time.Time `json:"validated_at,omitempty"`
	ValidatedBy        string     `json:"validated_by,omitempty"`
	AttendeeFirstName  string     `json:"attendee_first_name,omitempty"`
	AttendeeLastName   string     `json:"attendee_last_name,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	TicketTypeName     string     `json:"ticket_type_name,omitempty"` // Jointure
}

// ============================================
// Admin
// ============================================

type Admin struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	DisplayName  string     `json:"display_name"`
	Role         string     `json:"role"`
	IsActive     bool       `json:"is_active"`
	LastLogin    *time.Time `json:"last_login,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ============================================
// Request/Response DTOs
// ============================================

type CheckoutRequest struct {
	CustomerEmail     string        `json:"customer_email"`
	CustomerFirstName string        `json:"customer_first_name"`
	CustomerLastName  string        `json:"customer_last_name"`
	CustomerPhone     string        `json:"customer_phone,omitempty"`
	Items             []CheckoutItem `json:"items"`
}

type CheckoutItem struct {
	TicketTypeID string `json:"ticket_type_id"`
	CategoryID   string `json:"category_id,omitempty"`
	Quantity     int    `json:"quantity"`
}

type CheckoutResponse struct {
	OrderID     string `json:"order_id"`
	OrderNumber string `json:"order_number"`
	CheckoutURL string `json:"checkout_url"`
	TotalCents  int    `json:"total_cents"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token       string `json:"token"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type ValidateQRRequest struct {
	QRToken string `json:"qr_token"`
}

type ValidateQRResponse struct {
	Valid              bool   `json:"valid"`
	Message            string `json:"message"`
	TicketID           string `json:"ticket_id,omitempty"`
	AttendeeFirstName  string `json:"attendee_first_name,omitempty"`
	AttendeeLastName   string `json:"attendee_last_name,omitempty"`
	TicketTypeName     string `json:"ticket_type_name,omitempty"`
	OrderNumber        string `json:"order_number,omitempty"`
	AlreadyValidated   bool   `json:"already_validated,omitempty"`
}

type SalesStats struct {
	TotalOrders        int            `json:"total_orders"`
	TotalRevenueCents  int            `json:"total_revenue_cents"`
	TotalTicketsSold   int            `json:"total_tickets_sold"`
	TotalValidated     int            `json:"total_validated"`
	ByTicketType       []TicketTypeStat `json:"by_ticket_type"`
	RecentOrders       []Order        `json:"recent_orders"`
	SalesByDay         []DailySales   `json:"sales_by_day"`
}

type TicketTypeStat struct {
	TicketTypeID    string `json:"ticket_type_id"`
	Name            string `json:"name"`
	PriceCents      int    `json:"price_cents"`
	QuantityTotal   int    `json:"quantity_total"`
	QuantitySold    int    `json:"quantity_sold"`
	QuantityValidated int  `json:"quantity_validated"`
	RevenueCents    int    `json:"revenue_cents"`
}

type DailySales struct {
	Date          string `json:"date"`
	OrderCount    int    `json:"order_count"`
	TicketCount   int    `json:"ticket_count"`
	RevenueCents  int    `json:"revenue_cents"`
}

type OrderListParams struct {
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Status   OrderStatus `json:"status,omitempty"`
	Search   string      `json:"search,omitempty"`
}

type OrderListResponse struct {
	Orders     []Order `json:"orders"`
	TotalCount int     `json:"total_count"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
}

type CreateTicketTypeRequest struct {
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	PriceCents     int       `json:"price_cents"`
	QuantityTotal  int       `json:"quantity_total"`
	SaleStart      time.Time `json:"sale_start"`
	SaleEnd        time.Time `json:"sale_end"`
	MaxPerOrder    int       `json:"max_per_order"`
	AllowedDomains []string  `json:"allowed_domains"`
}

type CreateCategoryRequest struct {
	TicketTypeID   string   `json:"ticket_type_id"`
	Name           string   `json:"name"`
	Quantity       int      `json:"quantity"`
	AllowedDomains []string `json:"allowed_domains"`
}

type ReallocateCategoryRequest struct {
	SourceCategoryID string `json:"source_category_id"`
	TargetCategoryID string `json:"target_category_id"`
	Quantity         int    `json:"quantity"`
}

// TicketTypeWithCategories is returned by the public API for a given email
type TicketTypeForEmail struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description,omitempty"`
	PriceCents    int                `json:"price_cents"`
	QuantityTotal int                `json:"quantity_total"`
	QuantitySold  int                `json:"quantity_sold"`
	MaxPerOrder   int                `json:"max_per_order"`
	SaleStart     time.Time          `json:"sale_start"`
	SaleEnd       time.Time          `json:"sale_end"`
	IsActive      bool               `json:"is_active"`
	Categories    []CategoryForEmail `json:"categories"`
}

type CategoryForEmail struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	QuantityAllocated int    `json:"quantity_allocated"`
	QuantitySold      int    `json:"quantity_sold"`
}
