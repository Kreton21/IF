package models

import (
	"time"
)

// ============================================
// Ticket Types
// ============================================

type TicketType struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Description       string           `json:"description,omitempty"`
	PriceCents        int              `json:"price_cents"`
	QuantityTotal     int              `json:"quantity_total"`
	QuantitySold      int              `json:"quantity_sold"`
	SaleStart         time.Time        `json:"sale_start"`
	SaleEnd           time.Time        `json:"sale_end"`
	IsActive          bool             `json:"is_active"`
	IsMasked          bool             `json:"is_masked"`
	MaxPerOrder       int              `json:"max_per_order"`
	OneTicketPerEmail bool             `json:"one_ticket_per_email"`
	AllowedDomains    []string         `json:"allowed_domains"`
	Categories        []TicketCategory `json:"categories,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

type TicketCategory struct {
	ID                string    `json:"id"`
	TicketTypeID      string    `json:"ticket_type_id"`
	Name              string    `json:"name"`
	QuantityAllocated int       `json:"quantity_allocated"`
	QuantitySold      int       `json:"quantity_sold"`
	IsMasked          bool      `json:"is_masked"`
	IsCheckbox        bool      `json:"is_checkbox"`
	AllowedDomains    []string  `json:"allowed_domains"`
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
	DateOfBirth          string      `json:"date_of_birth,omitempty"`
	WantsCamping         bool        `json:"wants_camping,omitempty"`
	WantsRefundInsurance bool        `json:"wants_refund_insurance,omitempty"`
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
	ID                string     `json:"id"`
	OrderID           string     `json:"order_id"`
	TicketTypeID      string     `json:"ticket_type_id"`
	QRToken           string     `json:"qr_token"`
	QRCodeData        []byte     `json:"-"`
	IsValidated       bool       `json:"is_validated"`
	IsCamping         bool       `json:"is_camping"`
	ValidatedAt       *time.Time `json:"validated_at,omitempty"`
	ValidatedBy       string     `json:"validated_by,omitempty"`
	AttendeeFirstName string     `json:"attendee_first_name,omitempty"`
	AttendeeLastName  string     `json:"attendee_last_name,omitempty"`
	AttendeeEmail     string     `json:"attendee_email,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	TicketTypeName    string     `json:"ticket_type_name,omitempty"` // Jointure
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
	CustomerEmail     string         `json:"customer_email"`
	CustomerFirstName string         `json:"customer_first_name"`
	CustomerLastName  string         `json:"customer_last_name"`
	CustomerPhone     string         `json:"customer_phone,omitempty"`
	DateOfBirth       string         `json:"date_of_birth,omitempty"`
	ReferralCode      string         `json:"-"`
	ReferralVisitorID string         `json:"-"`
	WantsCamping      bool           `json:"wants_camping,omitempty"`
	WantsRefundInsurance bool         `json:"wants_refund_insurance,omitempty"`
	Items             []CheckoutItem `json:"items"`
}

type CheckoutItem struct {
	TicketTypeID string             `json:"ticket_type_id"`
	CategoryID   string             `json:"category_id,omitempty"`
	Quantity     int                `json:"quantity"`
	Attendees    []CheckoutAttendee `json:"attendees,omitempty"`
}

type CheckoutAttendee struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
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

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type SetStaffPasswordRequest struct {
	Username    string `json:"username"`
	NewPassword string `json:"new_password"`
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
	Valid             bool   `json:"valid"`
	Message           string `json:"message"`
	TicketID          string `json:"ticket_id,omitempty"`
	AttendeeFirstName string `json:"attendee_first_name,omitempty"`
	AttendeeLastName  string `json:"attendee_last_name,omitempty"`
	TicketTypeName    string `json:"ticket_type_name,omitempty"`
	OrderNumber       string `json:"order_number,omitempty"`
	AlreadyValidated  bool   `json:"already_validated,omitempty"`
	IsCamping         bool   `json:"is_camping,omitempty"`
	RideType          string `json:"ride_type,omitempty"`
	FromStation       string `json:"from_station,omitempty"`
	ToStation         string `json:"to_station,omitempty"`
	DepartureAt       string `json:"departure_at,omitempty"`
	ReturnDepartureAt string `json:"return_departure_at,omitempty"`
}

type SalesStats struct {
	TotalOrders       int              `json:"total_orders"`
	TotalRevenueCents int              `json:"total_revenue_cents"`
	TotalTicketsSold  int              `json:"total_tickets_sold"`
	TotalValidated    int              `json:"total_validated"`
	TotalCamping      int              `json:"total_camping"`
	TotalRefundInsurance int           `json:"total_refund_insurance"`
	TestEmailEnabled  bool             `json:"test_email_enabled"`
	ByTicketType      []TicketTypeStat `json:"by_ticket_type"`
	RecentOrders      []Order          `json:"recent_orders"`
	SalesByDay        []DailySales     `json:"sales_by_day"`
	SalesTimeline     map[string][]SalesTimelinePoint `json:"sales_timeline"`
}

type CampingClaimRequest struct {
	Email string `json:"email"`
}

type CampingClaimResponse struct {
	UpdatedTickets int    `json:"updated_tickets"`
	Message        string `json:"message"`
}

type TicketTypeStat struct {
	TicketTypeID      string `json:"ticket_type_id"`
	Name              string `json:"name"`
	PriceCents        int    `json:"price_cents"`
	QuantityTotal     int    `json:"quantity_total"`
	QuantitySold      int    `json:"quantity_sold"`
	QuantityValidated int    `json:"quantity_validated"`
	RevenueCents      int    `json:"revenue_cents"`
}

type DailySales struct {
	Date         string `json:"date"`
	OrderCount   int    `json:"order_count"`
	TicketCount  int    `json:"ticket_count"`
	RevenueCents int    `json:"revenue_cents"`
}

type DailyReferralSales struct {
	Date        string `json:"date"`
	ClickCount  int    `json:"click_count"`
	TicketCount int    `json:"ticket_count"`
}

type SalesTimelinePoint struct {
	Bucket      string `json:"bucket"`
	RevenueCents int   `json:"revenue_cents"`
	TicketCount int    `json:"ticket_count"`
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

type UpdateSuccessfulOrderRequest struct {
	CustomerFirstName    string `json:"customer_first_name"`
	CustomerLastName     string `json:"customer_last_name"`
	CustomerEmail        string `json:"customer_email"`
	WantsCamping         bool   `json:"wants_camping"`
	WantsRefundInsurance bool   `json:"wants_refund_insurance"`
}

type CreateTicketTypeRequest struct {
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	PriceCents        int       `json:"price_cents"`
	QuantityTotal     int       `json:"quantity_total"`
	SaleStart         time.Time `json:"sale_start"`
	SaleEnd           time.Time `json:"sale_end"`
	MaxPerOrder       int       `json:"max_per_order"`
	OneTicketPerEmail bool      `json:"one_ticket_per_email"`
	AllowedDomains    []string  `json:"allowed_domains"`
}

type UpdateTicketTypeRequest struct {
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	PriceCents        int       `json:"price_cents"`
	QuantityTotal     int       `json:"quantity_total"`
	SaleStart         time.Time `json:"sale_start"`
	SaleEnd           time.Time `json:"sale_end"`
	OneTicketPerEmail bool      `json:"one_ticket_per_email"`
	AllowedDomains    []string  `json:"allowed_domains"`
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
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Description       string             `json:"description,omitempty"`
	PriceCents        int                `json:"price_cents"`
	MaxPerOrder       int                `json:"max_per_order"`
	MaxSelectable     int                `json:"max_selectable"`
	OneTicketPerEmail bool               `json:"one_ticket_per_email"`
	SaleStart         time.Time          `json:"sale_start"`
	SaleEnd           time.Time          `json:"sale_end"`
	IsActive          bool               `json:"is_active"`
	IsAvailable       bool               `json:"is_available"`
	Categories        []CategoryForEmail `json:"categories"`
}

type CategoryForEmail struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IsCheckbox  bool   `json:"is_checkbox"`
	IsAvailable bool   `json:"is_available"`
}

type PublicBusDeparture struct {
	ID            string    `json:"id"`
	StationID     string    `json:"station_id"`
	Direction     string    `json:"direction"`
	DepartureTime time.Time `json:"departure_time"`
	PriceCents    int       `json:"price_cents"`
	IsActive      bool      `json:"is_active"`
}

type PublicBusOptionsResponse struct {
	Stations           []BusStation         `json:"stations"`
	OutboundDepartures []PublicBusDeparture `json:"outbound_departures"`
	ReturnDepartures   []PublicBusDeparture `json:"return_departures"`
}

type BusStation struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BusDeparture struct {
	ID            string    `json:"id"`
	StationID     string    `json:"station_id"`
	StationName   string    `json:"station_name,omitempty"`
	Direction     string    `json:"direction"`
	DepartureTime time.Time `json:"departure_time"`
	PriceCents    int       `json:"price_cents"`
	Capacity      int       `json:"capacity"`
	Sold          int       `json:"sold"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type BusOptionsResponse struct {
	Stations           []BusStation   `json:"stations"`
	OutboundDepartures []BusDeparture `json:"outbound_departures"`
	ReturnDepartures   []BusDeparture `json:"return_departures"`
}

type BusCheckoutRequest struct {
	CustomerEmail       string `json:"customer_email"`
	CustomerFirstName   string `json:"customer_first_name"`
	CustomerLastName    string `json:"customer_last_name"`
	CustomerPhone       string `json:"customer_phone,omitempty"`
	TripType            string `json:"trip_type,omitempty"` // outbound | return | round_trip
	FromStationID       string `json:"from_station_id,omitempty"`
	OutboundDepartureID string `json:"outbound_departure_id,omitempty"`
	RoundTrip           bool   `json:"round_trip,omitempty"` // rétrocompat
	ReturnDepartureID   string `json:"return_departure_id,omitempty"`
	ReturnStationID     string `json:"return_station_id,omitempty"`
}

type CreateBusStationRequest struct {
	Name string `json:"name"`
}

type CreateBusDepartureRequest struct {
	StationID     string    `json:"station_id"`
	Direction     string    `json:"direction"`
	DepartureTime time.Time `json:"departure_time"`
	PriceCents    int       `json:"price_cents"`
	Capacity      int       `json:"capacity"`
	IsActive      bool      `json:"is_active"`
}

type UpdateBusDepartureRequest struct {
	StationID     string    `json:"station_id"`
	Direction     string    `json:"direction"`
	DepartureTime time.Time `json:"departure_time"`
	PriceCents    int       `json:"price_cents"`
	Capacity      int       `json:"capacity"`
	IsActive      bool      `json:"is_active"`
}

type BusTicketAdminRow struct {
	TicketID            string     `json:"ticket_id"`
	OrderNumber         string     `json:"order_number"`
	OrderTotalCents     int        `json:"order_total_cents"`
	CustomerFirstName   string     `json:"customer_first_name"`
	CustomerLastName    string     `json:"customer_last_name"`
	CustomerEmail       string     `json:"customer_email"`
	FromStation         string     `json:"from_station"`
	ToStation           string     `json:"to_station"`
	DepartureTime       time.Time  `json:"departure_time"`
	ReturnDepartureTime *time.Time `json:"return_departure_time,omitempty"`
	IsRoundTrip         bool       `json:"is_round_trip"`
	IsValidated         bool       `json:"is_validated"`
	CreatedAt           time.Time  `json:"created_at"`
}

type CreateReferralLinkRequest struct {
	Name       string `json:"name"`
	CustomCode string `json:"custom_code"`
}

type ReferralLinkRow struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Code             string    `json:"code"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	ClickCount       int       `json:"click_count"`
	UniqueVisitors   int       `json:"unique_visitors"`
	ConvertedOrders  int       `json:"converted_orders"`
	ConvertedTickets int       `json:"converted_tickets"`
	ConvertedRevenue int       `json:"converted_revenue_cents"`
	DailySalesByDay  []DailyReferralSales `json:"daily_sales_by_day,omitempty"`
	ShareURL         string    `json:"share_url,omitempty"`
}

type CreateReferralLinkResponse struct {
	Link ReferralLinkRow `json:"link"`
}

type ReferralPublicInfo struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

// ══════════════════════════════════════
// Analytics
// ══════════════════════════════════════

type AnalyticsEvent struct {
	SessionID string           `json:"session_id"`
	Type      string           `json:"type"` // "session_start", "click", "session_end"
	Page      string           `json:"page"`
	Target    string           `json:"target,omitempty"`
	Referrer  string           `json:"referrer,omitempty"`
	Duration  int64            `json:"duration_ms,omitempty"`
}

type AnalyticsKPI struct {
	TotalSessions      int                    `json:"total_sessions"`
	TotalClicks        int                    `json:"total_clicks"`
	AvgSessionDuration float64                `json:"avg_session_duration_s"`
	ClicksTimeline     []AnalyticsTimePoint   `json:"clicks_timeline"`
	SessionsTimeline   []AnalyticsTimePoint   `json:"sessions_timeline"`
	TopPages           []AnalyticsPageStat    `json:"top_pages"`
	TicketOrigins      []AnalyticsTicketOrigin `json:"ticket_origins"`
}

type AnalyticsTimePoint struct {
	Bucket string `json:"bucket"`
	Count  int    `json:"count"`
}

type AnalyticsPageStat struct {
	Page     string `json:"page"`
	Sessions int    `json:"sessions"`
	Clicks   int    `json:"clicks"`
}

type AnalyticsTicketOrigin struct {
	Domain      string `json:"domain"`
	Category    string `json:"category"`
	TicketType  string `json:"ticket_type"`
	TicketCount int    `json:"ticket_count"`
}
