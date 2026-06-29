package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/adham/hotel-qr-ordering/internal/auth"
	"github.com/adham/hotel-qr-ordering/internal/model"
	"github.com/adham/hotel-qr-ordering/internal/repository"
)

type HotelService struct {
	dbRepo    *repository.PostgresRepository
	redisRepo *repository.RedisRepository
}

func NewHotelService(db *repository.PostgresRepository, redis *repository.RedisRepository) *HotelService {
	return &HotelService{
		dbRepo:    db,
		redisRepo: redis,
	}
}

// --- SaaS Auth & Config Services ---

func (s *HotelService) SignupAdmin(ctx context.Context, email, password, propName string) (*model.User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user, _, err := s.dbRepo.CreateUserAndProperty(ctx, email, string(hashed), propName)
	if err != nil {
		return nil, fmt.Errorf("failed to create user and property: %w", err)
	}
	return user, nil
}

func (s *HotelService) LoginAdmin(ctx context.Context, email, password string) (string, error) {
	user, err := s.dbRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return "", errors.New("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", errors.New("invalid email or password")
	}

	token, err := auth.GenerateToken(user.ID, user.PropertyID)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

func (s *HotelService) ToggleService(ctx context.Context, propID, srvType string, isEnabled bool) error {
	err := s.dbRepo.SetPropertyService(ctx, propID, srvType, isEnabled)
	if err != nil {
		return fmt.Errorf("failed to toggle service: %w", err)
	}

	// Flush Redis cache for this property
	if err := s.redisRepo.InvalidateMenuCache(ctx, propID); err != nil {
		log.Printf("Warning: Failed to invalidate cache after toggle: %v", err)
	}

	// Publish real-time event to sync clients
	eventPayload := map[string]interface{}{
		"property_id":  propID,
		"service_type": srvType,
		"is_enabled":   isEnabled,
	}
	if err := s.redisRepo.PublishOrderEvent(ctx, "service_toggled", eventPayload); err != nil {
		log.Printf("Warning: Failed to publish service_toggled event: %v", err)
	}

	return nil
}

// --- Client Bootstrap & Catalog ---

// --- Guest Stay Session Negotiation ---

func (s *HotelService) NegotiateGuestSession(ctx context.Context, roomStaticToken, oldGuestToken string) (string, error) {
	room, err := s.dbRepo.GetRoomByToken(ctx, roomStaticToken)
	if err != nil {
		return "", fmt.Errorf("invalid room static token: %w", err)
	}

	// If the caller has a known session token, try to honor it first.
	if oldGuestToken != "" {
		session, err := s.dbRepo.GetGuestSessionByToken(ctx, oldGuestToken)
		if err == nil && session.RoomID == room.ID {
			switch session.Status {
			case "active":
				newExpiry := time.Now().Add(12 * time.Hour)
				_ = s.dbRepo.ExtendGuestSession(ctx, session.ID, newExpiry)
				return oldGuestToken, nil
			case "archived":
				// Read-only checkout screen for up to 48 hours after archive
				if time.Now().Before(session.ExpiresAt.Add(48 * time.Hour)) {
					return oldGuestToken, nil
				}
			}
		}
	}

	// No valid token from this browser — check if another device has an active session
	// for this room (same guest, second device). Join it instead of creating a new one.
	existingSession, err := s.dbRepo.GetActiveGuestSessionForRoom(ctx, room.ID)
	if err == nil && existingSession != nil {
		return existingSession.SessionToken, nil
	}

	// Truly no active session → new guest check-in.
	_ = s.dbRepo.InvalidateAllGuestSessionsForRoom(ctx, room.ID)
	newToken := uuid.New().String()
	expiresAt := time.Now().Add(12 * time.Hour)
	_, err = s.dbRepo.CreateGuestSession(ctx, room.ID, newToken, expiresAt)
	if err != nil {
		return "", fmt.Errorf("failed to create guest session: %w", err)
	}
	return newToken, nil
}

func (s *HotelService) CheckoutRoom(ctx context.Context, roomID, propertyID string) error {
	room, err := s.dbRepo.GetRoomByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("room not found: %w", err)
	}
	if room.PropertyID != propertyID {
		return fmt.Errorf("room does not belong to this property")
	}

	if err := s.dbRepo.InvalidateAllGuestSessionsForRoom(ctx, roomID); err != nil {
		return fmt.Errorf("failed to checkout room: %w", err)
	}

	eventPayload := map[string]interface{}{
		"property_id": propertyID,
		"room_id":     roomID,
		"room_number": room.RoomNumber,
		"action":      "guest_checked_out",
	}
	_ = s.redisRepo.PublishOrderEvent(ctx, "room_updated", eventPayload)
	return nil
}

// --- Client Bootstrap & Catalog ---

func (s *HotelService) GetClientBootstrap(ctx context.Context, sessionToken string) (*model.BootstrapResponse, *model.Room, error) {
	session, err := s.dbRepo.GetGuestSessionByToken(ctx, sessionToken)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid guest session: %w", err)
	}

	isExpired := false
	if session.Status != "active" || time.Now().After(session.ExpiresAt) {
		isExpired = true
	} else {
		// Refresh sliding window on successful bootstrap activity (12 hours)
		_ = s.dbRepo.ExtendGuestSession(ctx, session.ID, time.Now().Add(12*time.Hour))
	}

	room, err := s.dbRepo.GetRoomByID(ctx, session.RoomID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load room: %w", err)
	}

	propID := room.PropertyID

	prop, err := s.dbRepo.GetProperty(ctx, propID)
	if err != nil {
		return nil, nil, err
	}

	services, err := s.dbRepo.GetPropertyServices(ctx, propID)
	if err != nil {
		return nil, nil, err
	}

	catalog, err := s.dbRepo.GetCatalogItems(ctx, propID, "")
	if err != nil {
		return nil, nil, err
	}

	// Filter catalog based on active services
	activeServices := make(map[string]bool)
	for _, srv := range services {
		if srv.IsEnabled {
			activeServices[srv.ServiceType] = true
		}
	}

	var filteredCatalog []model.CatalogItem
	for _, item := range catalog {
		if activeServices[item.ServiceType] && item.IsAvailable {
			filteredCatalog = append(filteredCatalog, item)
		}
	}

	res := &model.BootstrapResponse{
		Property:   *prop,
		Services:   services,
		Catalog:    filteredCatalog,
		RoomNumber: room.RoomNumber,
		IsExpired:  isExpired,
	}

	return res, room, nil
}

// --- Orders & Operations ---

func (s *HotelService) PlaceOrder(ctx context.Context, req *model.OrderRequest) (*model.Order, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("cannot place an order with empty items")
	}

	session, err := s.dbRepo.GetGuestSessionByToken(ctx, req.RoomToken)
	if err != nil {
		return nil, fmt.Errorf("invalid guest session: %w", err)
	}

	if session.Status != "active" || time.Now().After(session.ExpiresAt) {
		return nil, errors.New("session_expired_cannot_order")
	}

	// Refresh sliding window on active ordering
	_ = s.dbRepo.ExtendGuestSession(ctx, session.ID, time.Now().Add(12*time.Hour))

	room, err := s.dbRepo.GetRoomByID(ctx, session.RoomID)
	if err != nil {
		return nil, fmt.Errorf("failed to load room: %w", err)
	}

	services, err := s.dbRepo.GetPropertyServices(ctx, room.PropertyID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch property services: %w", err)
	}

	activeServices := make(map[string]bool)
	for _, srv := range services {
		if srv.IsEnabled {
			activeServices[srv.ServiceType] = true
		}
	}

	var totalAmount float64
	var orderItems []model.OrderItem

	for _, reqItem := range req.Items {
		catalogItem, err := s.dbRepo.GetCatalogItemByID(ctx, reqItem.CatalogItemID)
		if err != nil {
			return nil, fmt.Errorf("catalog item %s not found: %w", reqItem.CatalogItemID, err)
		}

		if !catalogItem.IsAvailable {
			return nil, fmt.Errorf("item '%s' is unavailable", catalogItem.Name)
		}

		if catalogItem.PropertyID != room.PropertyID {
			return nil, fmt.Errorf("item '%s' belongs to a different property", catalogItem.Name)
		}

		if !activeServices[catalogItem.ServiceType] {
			return nil, fmt.Errorf("service module '%s' is currently disabled by hotel administration", catalogItem.ServiceType)
		}

		itemTotal := catalogItem.Price * float64(reqItem.Quantity)
		totalAmount += itemTotal

		orderItems = append(orderItems, model.OrderItem{
			CatalogItemID: catalogItem.ID,
			ItemName:      catalogItem.Name,
			ServiceType:   catalogItem.ServiceType,
			Quantity:      reqItem.Quantity,
			Price:         catalogItem.Price,
			Attributes:    reqItem.Attributes,
		})
	}

	order := &model.Order{
		RoomID:      room.ID,
		RoomNumber:  room.RoomNumber,
		PropertyID:  room.PropertyID,
		QRToken:     room.QRToken,
		SessionID:   session.ID,
		GroupID:     req.GroupID,
		TotalAmount: totalAmount,
		Items:       orderItems,
	}

	if err := s.dbRepo.CreateOrder(ctx, order); err != nil {
		return nil, fmt.Errorf("failed to save order: %w", err)
	}

	if err := s.redisRepo.PublishOrderEvent(ctx, "order_created", order); err != nil {
		log.Printf("Warning: Failed to publish order_created event to Redis: %v", err)
	}

	return order, nil
}

func (s *HotelService) GetActiveOrders(ctx context.Context, propID string) ([]model.Order, error) {
	return s.dbRepo.GetOrdersByProperty(ctx, propID)
}

func (s *HotelService) GetClientOrders(ctx context.Context, sessionToken string, all bool) ([]model.Order, error) {
	session, err := s.dbRepo.GetGuestSessionByToken(ctx, sessionToken)
	if err != nil {
		return nil, fmt.Errorf("invalid guest session: %w", err)
	}

	if all {
		return s.dbRepo.GetOrdersBySessionID(ctx, session.ID)
	}
	return s.dbRepo.GetActiveOrdersBySessionID(ctx, session.ID)
}

func (s *HotelService) UpdateOrderStatus(ctx context.Context, orderID string, status model.OrderStatus) (*model.Order, error) {
	order, err := s.dbRepo.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if len(order.Items) == 0 && (status == model.StatusAccepted || status == model.StatusCompleted) {
		return nil, errors.New("cannot accept or complete an order with no items")
	}

	if err := s.dbRepo.UpdateOrderStatus(ctx, orderID, status); err != nil {
		return nil, fmt.Errorf("failed to update status: %w", err)
	}

	// Refetch to return the updated status
	order, err = s.dbRepo.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if err := s.redisRepo.PublishOrderEvent(ctx, "order_updated", order); err != nil {
		log.Printf("Warning: Failed to publish event: %v", err)
	}

	return order, nil
}

func (s *HotelService) RemoveItemFromOrder(ctx context.Context, orderID string, itemID string) (*model.Order, error) {
	if err := s.dbRepo.RemoveOrderItem(ctx, orderID, itemID); err != nil {
		return nil, fmt.Errorf("failed to remove item: %w", err)
	}

	order, err := s.dbRepo.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}

	// If no items are left in the order, mark it as cancelled (rejected) automatically
	if len(order.Items) == 0 {
		if err := s.dbRepo.UpdateOrderStatus(ctx, orderID, model.StatusCancelled); err != nil {
			return nil, fmt.Errorf("failed to cancel empty order: %w", err)
		}
		// Refetch the order to get the updated status
		order, err = s.dbRepo.GetOrder(ctx, orderID)
		if err != nil {
			return nil, err
		}
	}

	if err := s.redisRepo.PublishOrderEvent(ctx, "order_updated", order); err != nil {
		log.Printf("Warning: Failed to publish event: %v", err)
	}

	return order, nil
}

func (s *HotelService) GetPropertyServices(ctx context.Context, propID string) ([]model.PropertyService, error) {
	return s.dbRepo.GetPropertyServices(ctx, propID)
}

func (s *HotelService) GetCatalogItems(ctx context.Context, propID, search string) ([]model.CatalogItem, error) {
	return s.dbRepo.GetCatalogItems(ctx, propID, search)
}

func (s *HotelService) CreateCatalogItem(ctx context.Context, item *model.CatalogItem) error {
	if err := s.dbRepo.CreateCatalogItem(ctx, item); err != nil {
		return err
	}
	_ = s.redisRepo.InvalidateMenuCache(ctx, item.PropertyID)

	// Publish real-time event to sync clients
	eventPayload := map[string]interface{}{
		"property_id": item.PropertyID,
		"item_id":     item.ID,
	}
	if err := s.redisRepo.PublishOrderEvent(ctx, "catalog_updated", eventPayload); err != nil {
		log.Printf("Warning: Failed to publish catalog_updated event: %v", err)
	}

	return nil
}

func (s *HotelService) UpdateCatalogItem(ctx context.Context, item *model.CatalogItem) error {
	if err := s.dbRepo.UpdateCatalogItem(ctx, item); err != nil {
		return err
	}
	_ = s.redisRepo.InvalidateMenuCache(ctx, item.PropertyID)

	// Publish real-time event to sync clients
	eventPayload := map[string]interface{}{
		"property_id": item.PropertyID,
		"item_id":     item.ID,
	}
	if err := s.redisRepo.PublishOrderEvent(ctx, "catalog_updated", eventPayload); err != nil {
		log.Printf("Warning: Failed to publish catalog_updated event: %v", err)
	}

	return nil
}

func (s *HotelService) DeleteCatalogItem(ctx context.Context, itemID, propID string) error {
	if err := s.dbRepo.DeleteCatalogItem(ctx, itemID, propID); err != nil {
		return err
	}
	_ = s.redisRepo.InvalidateMenuCache(ctx, propID)

	// Publish real-time event to sync clients
	eventPayload := map[string]interface{}{
		"property_id": propID,
		"item_id":     itemID,
	}
	if err := s.redisRepo.PublishOrderEvent(ctx, "catalog_updated", eventPayload); err != nil {
		log.Printf("Warning: Failed to publish catalog_updated event: %v", err)
	}

	return nil
}

func (s *HotelService) CreateRoom(ctx context.Context, propertyID, roomNumber, floor, building string) (*model.Room, error) {
	qrToken := uuid.New().String()
	return s.dbRepo.CreateRoom(ctx, propertyID, roomNumber, floor, building, qrToken)
}

func (s *HotelService) GetRooms(ctx context.Context, propertyID, search string) ([]model.Room, error) {
	return s.dbRepo.GetRoomsByProperty(ctx, propertyID, search)
}

func (s *HotelService) RotateRoomToken(ctx context.Context, roomID string, propertyID string) (string, error) {
	newToken := uuid.New().String()
	if err := s.dbRepo.UpdateRoomQRToken(ctx, roomID, propertyID, newToken); err != nil {
		return "", err
	}

	// Publish dynamic room_updated event so any guest browser on the old token can reload and fail
	eventPayload := map[string]interface{}{
		"property_id": propertyID,
		"room_id":     roomID,
		"action":      "token_rotated",
	}
	_ = s.redisRepo.PublishOrderEvent(ctx, "room_updated", eventPayload)

	return newToken, nil
}
