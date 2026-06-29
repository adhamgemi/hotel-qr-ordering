package model

import (
	"time"
)

// OrderStatus defines the state of an order
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusAccepted  OrderStatus = "accepted"
	StatusCompleted OrderStatus = "completed"
	StatusCancelled OrderStatus = "cancelled"
)

// User represents a hotel admin
type User struct {
	ID           string    `json:"id"`
	PropertyID   string    `json:"property_id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Never leak the hash
	CreatedAt    time.Time `json:"created_at"`
}

// Property represents a hotel property
type Property struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// PropertyService represents an enabled module for a property
type PropertyService struct {
	ID          string    `json:"id"`
	PropertyID  string    `json:"property_id"`
	ServiceType string    `json:"service_type"` // e.g., fnb, laundry, housekeeping
	IsEnabled   bool      `json:"is_enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// Room represents a hotel room associated with a property
type Room struct {
	ID         string    `json:"id"`
	PropertyID string    `json:"property_id"`
	RoomNumber string    `json:"room_number"`
	Floor      string    `json:"floor"`
	Building   string    `json:"building"`
	QRToken    string    `json:"qr_token"`
	CreatedAt  time.Time `json:"created_at"`
}

// CatalogItem represents any service offering (Food, Laundry, etc)
type CatalogItem struct {
	ID          string                 `json:"id"`
	PropertyID  string                 `json:"property_id"`
	ServiceType string                 `json:"service_type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Price       float64                `json:"price"`
	IsAvailable bool                   `json:"is_available"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"` // JSONB payload
	CreatedAt   time.Time              `json:"created_at"`
}

// GuestSession represents an active or archived stay session for a room
type GuestSession struct {
	ID           string    `json:"id"`
	RoomID       string    `json:"room_id"`
	SessionToken string    `json:"session_token"`
	Status       string    `json:"status"` // active, archived
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type NegotiateSessionRequest struct {
	RoomStaticToken string `json:"room_static_token" binding:"required"`
	OldGuestToken   string `json:"old_guest_token,omitempty"`
}

type NegotiateSessionResponse struct {
	SessionToken string `json:"session_token"`
	Status       string `json:"status"` // active, archived
}

// Order represents an order placed by a guest in a room
type Order struct {
	ID          string      `json:"id"`
	RoomID      string      `json:"room_id"`
	RoomNumber  string      `json:"room_number"` // Loaded dynamically
	PropertyID  string      `json:"property_id,omitempty"` // Loaded dynamically
	QRToken     string      `json:"qr_token,omitempty"`
	SessionID   string      `json:"session_id,omitempty"`
	GroupID     string      `json:"group_id,omitempty"`
	Status      OrderStatus `json:"status"`
	TotalAmount float64     `json:"total_amount"`
	CreatedAt   time.Time   `json:"created_at"`
	Items       []OrderItem `json:"items,omitempty"`
}

// OrderItem represents a single item and quantity inside an order
type OrderItem struct {
	ID            string                 `json:"id"`
	OrderID       string                 `json:"order_id"`
	CatalogItemID string                 `json:"catalog_item_id"`
	ItemName      string                 `json:"item_name,omitempty"` // Loaded dynamically
	ServiceType   string                 `json:"service_type,omitempty"` // Loaded dynamically
	Quantity      int                    `json:"quantity"`
	Price         float64                `json:"price"` // Price at time of ordering
	Attributes    map[string]interface{} `json:"attributes,omitempty"` // Specific selections
}

// --- Request Payloads ---

type AuthRequest struct {
	Email        string `json:"email" binding:"required,email"`
	Password     string `json:"password" binding:"required,min=6"`
	PropertyName string `json:"property_name"` // Required only for signup
}

type OrderItemRequest struct {
	CatalogItemID string                 `json:"catalog_item_id" binding:"required"`
	Quantity      int                    `json:"quantity" binding:"required,min=1"`
	Attributes    map[string]interface{} `json:"attributes,omitempty"`
}

type OrderRequest struct {
	RoomToken string             `json:"room_token" binding:"required"`
	GroupID   string             `json:"group_id,omitempty"`
	Items     []OrderItemRequest `json:"items" binding:"required,dive"`
}

type StatusUpdateRequest struct {
	Status OrderStatus `json:"status" binding:"required"`
}

// CreateRoomRequest used when admin adds a new room
type CreateRoomRequest struct {
	RoomNumber string `json:"room_number" binding:"required"`
	Floor      string `json:"floor"`
	Building   string `json:"building"`
}

// ToggleServiceRequest used to enable/disable modules
type ToggleServiceRequest struct {
	ServiceType string `json:"service_type" binding:"required"`
	IsEnabled   bool   `json:"is_enabled"`
}

// BootstrapResponse used by the guest portal to render UI
type BootstrapResponse struct {
	Property   Property          `json:"property"`
	Services   []PropertyService `json:"services"`
	Catalog    []CatalogItem     `json:"catalog"`
	RoomNumber string            `json:"room_number,omitempty"`
	IsExpired  bool              `json:"is_expired"`
}

// WSEvent represents the payload broadcast over WebSockets
type WSEvent struct {
	Type    string      `json:"type"` // e.g. "order_created", "order_updated"
	Payload interface{} `json:"payload"`
}
