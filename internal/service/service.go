package service

import (
	"context"
	"errors"
	"fmt"
	"log"

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

func (s *HotelService) GetClientBootstrap(ctx context.Context, roomNumber string) (*model.BootstrapResponse, *model.Room, error) {
	room, err := s.dbRepo.GetRoomByNumber(ctx, roomNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid room number: %w", err)
	}

	propID := room.PropertyID

	// NOTE: We could cache the entire BootstrapResponse in Redis instead of just the menu.
	// For now, we fetch from DB.
	prop, err := s.dbRepo.GetProperty(ctx, propID)
	if err != nil {
		return nil, nil, err
	}

	services, err := s.dbRepo.GetPropertyServices(ctx, propID)
	if err != nil {
		return nil, nil, err
	}

	catalog, err := s.dbRepo.GetCatalogItems(ctx, propID)
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
		Property: *prop,
		Services: services,
		Catalog:  filteredCatalog,
	}

	return res, room, nil
}

// --- Orders & Operations ---

func (s *HotelService) PlaceOrder(ctx context.Context, req *model.OrderRequest) (*model.Order, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("cannot place an order with empty items")
	}

	room, err := s.dbRepo.GetRoomByNumber(ctx, req.RoomNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid room number: %w", err)
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

		itemTotal := catalogItem.Price * float64(reqItem.Quantity)
		totalAmount += itemTotal

		orderItems = append(orderItems, model.OrderItem{
			CatalogItemID: catalogItem.ID,
			ItemName:      catalogItem.Name,
			ServiceType:   catalogItem.ServiceType,
			Quantity:      reqItem.Quantity,
			Price:         catalogItem.Price,
			Attributes:    reqItem.Attributes, // Pass JSONB attributes down
		})
	}

	order := &model.Order{
		RoomID:      room.ID,
		RoomNumber:  room.RoomNumber,
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

func (s *HotelService) GetClientOrders(ctx context.Context, roomNumber string) ([]model.Order, error) {
	return s.dbRepo.GetActiveOrdersByRoom(ctx, roomNumber)
}

func (s *HotelService) UpdateOrderStatus(ctx context.Context, orderID string, status model.OrderStatus) (*model.Order, error) {
	if err := s.dbRepo.UpdateOrderStatus(ctx, orderID, status); err != nil {
		return nil, fmt.Errorf("failed to update status: %w", err)
	}

	order, err := s.dbRepo.GetOrder(ctx, orderID)
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

	if err := s.redisRepo.PublishOrderEvent(ctx, "order_updated", order); err != nil {
		log.Printf("Warning: Failed to publish event: %v", err)
	}

	return order, nil
}

func (s *HotelService) GetPropertyServices(ctx context.Context, propID string) ([]model.PropertyService, error) {
	return s.dbRepo.GetPropertyServices(ctx, propID)
}

func (s *HotelService) GetCatalogItems(ctx context.Context, propID string) ([]model.CatalogItem, error) {
	return s.dbRepo.GetCatalogItems(ctx, propID)
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
