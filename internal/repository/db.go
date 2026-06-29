package repository

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adham/hotel-qr-ordering/internal/model"
)

type PostgresRepository struct {
	Pool *pgxpool.Pool
}

func NewPostgresRepository(connStr string) (*PostgresRepository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	log.Println("Successfully connected to PostgreSQL")
	return &PostgresRepository{Pool: pool}, nil
}

func (r *PostgresRepository) Close() {
	if r.Pool != nil {
		r.Pool.Close()
	}
}

func (r *PostgresRepository) RunMigrations(migrationsDir string) error {
	ctx := context.Background()

	schemaPath := filepath.Join(migrationsDir, "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema.sql: %w", err)
	}
	if _, err := r.Pool.Exec(ctx, string(schemaSQL)); err != nil {
		return fmt.Errorf("failed to execute schema.sql: %w", err)
	}

	seedPath := filepath.Join(migrationsDir, "seed.sql")
	seedSQL, err := os.ReadFile(seedPath)
	if err != nil {
		return fmt.Errorf("failed to read seed.sql: %w", err)
	}
	if _, err := r.Pool.Exec(ctx, string(seedSQL)); err != nil {
		return fmt.Errorf("failed to execute seed.sql: %w", err)
	}

	return nil
}

// --- Multi-Tenant & Auth Methods ---

func (r *PostgresRepository) CreateUserAndProperty(ctx context.Context, email, passHash, propName string) (*model.User, *model.Property, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	propID := uuid.New().String()
	propQuery := `INSERT INTO properties (id, name) VALUES ($1, $2) RETURNING created_at`
	var propCreated time.Time
	if err := tx.QueryRow(ctx, propQuery, propID, propName).Scan(&propCreated); err != nil {
		return nil, nil, err
	}

	userID := uuid.New().String()
	userQuery := `INSERT INTO users (id, property_id, email, password_hash) VALUES ($1, $2, $3, $4) RETURNING created_at`
	var userCreated time.Time
	if err := tx.QueryRow(ctx, userQuery, userID, propID, email, passHash).Scan(&userCreated); err != nil {
		return nil, nil, err
	}

	// Add default services
	services := []string{"fnb", "housekeeping", "laundry", "maintenance", "concierge"}
	srvQuery := `INSERT INTO property_services (property_id, service_type, is_enabled) VALUES ($1, $2, TRUE)`
	for _, srv := range services {
		if _, err := tx.Exec(ctx, srvQuery, propID, srv); err != nil {
			return nil, nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	return &model.User{ID: userID, PropertyID: propID, Email: email, CreatedAt: userCreated},
		&model.Property{ID: propID, Name: propName, CreatedAt: propCreated}, nil
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT id, property_id, email, password_hash, created_at FROM users WHERE email = $1`
	var u model.User
	err := r.Pool.QueryRow(ctx, query, email).Scan(&u.ID, &u.PropertyID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *PostgresRepository) GetProperty(ctx context.Context, id string) (*model.Property, error) {
	query := `SELECT id, name, created_at FROM properties WHERE id = $1`
	var p model.Property
	err := r.Pool.QueryRow(ctx, query, id).Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PostgresRepository) GetPropertyServices(ctx context.Context, propID string) ([]model.PropertyService, error) {
	query := `SELECT id, property_id, service_type, is_enabled, created_at FROM property_services WHERE property_id = $1`
	rows, err := r.Pool.Query(ctx, query, propID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var svcs []model.PropertyService
	for rows.Next() {
		var s model.PropertyService
		if err := rows.Scan(&s.ID, &s.PropertyID, &s.ServiceType, &s.IsEnabled, &s.CreatedAt); err != nil {
			return nil, err
		}
		svcs = append(svcs, s)
	}
	return svcs, nil
}

func (r *PostgresRepository) SetPropertyService(ctx context.Context, propID, srvType string, isEnabled bool) error {
	query := `
		INSERT INTO property_services (id, property_id, service_type, is_enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (property_id, service_type) DO UPDATE SET is_enabled = $4
	`
	_, err := r.Pool.Exec(ctx, query, uuid.New().String(), propID, srvType, isEnabled)
	return err
}

// --- Core Booking & Catalog Methods ---

func (r *PostgresRepository) CreateRoom(ctx context.Context, propertyID, roomNumber, floor, building, qrToken string) (*model.Room, error) {
	query := `
		INSERT INTO rooms (id, property_id, room_number, floor, building, qr_token)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, property_id, room_number, floor, building, qr_token, created_at`
	id := uuid.New().String()
	var rm model.Room
	err := r.Pool.QueryRow(ctx, query, id, propertyID, roomNumber, floor, building, qrToken).
		Scan(&rm.ID, &rm.PropertyID, &rm.RoomNumber, &rm.Floor, &rm.Building, &rm.QRToken, &rm.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (r *PostgresRepository) GetRoomByNumber(ctx context.Context, roomNumber string) (*model.Room, error) {
	query := `SELECT id, property_id, room_number, floor, building, qr_token, created_at FROM rooms WHERE room_number = $1`
	var rm model.Room
	err := r.Pool.QueryRow(ctx, query, roomNumber).Scan(&rm.ID, &rm.PropertyID, &rm.RoomNumber, &rm.Floor, &rm.Building, &rm.QRToken, &rm.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (r *PostgresRepository) GetRoomByToken(ctx context.Context, token string) (*model.Room, error) {
	query := `SELECT id, property_id, room_number, floor, building, qr_token, created_at FROM rooms WHERE qr_token = $1`
	var rm model.Room
	err := r.Pool.QueryRow(ctx, query, token).Scan(&rm.ID, &rm.PropertyID, &rm.RoomNumber, &rm.Floor, &rm.Building, &rm.QRToken, &rm.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (r *PostgresRepository) GetRoomsByProperty(ctx context.Context, propertyID, search string) ([]model.Room, error) {
	query := `
		SELECT id, property_id, room_number, floor, building, qr_token, created_at
		FROM rooms
		WHERE property_id = $1
		  AND ($2 = '' OR room_number ILIKE '%' || $2 || '%'
		                OR floor      ILIKE '%' || $2 || '%'
		                OR building   ILIKE '%' || $2 || '%')
		ORDER BY building, floor, room_number`
	rows, err := r.Pool.Query(ctx, query, propertyID, search)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []model.Room
	for rows.Next() {
		var rm model.Room
		if err := rows.Scan(&rm.ID, &rm.PropertyID, &rm.RoomNumber, &rm.Floor, &rm.Building, &rm.QRToken, &rm.CreatedAt); err != nil {
			return nil, err
		}
		rooms = append(rooms, rm)
	}
	return rooms, nil
}

func (r *PostgresRepository) UpdateRoomQRToken(ctx context.Context, roomID string, propertyID string, newToken string) error {
	query := `UPDATE rooms SET qr_token = $1 WHERE id = $2 AND property_id = $3`
	tag, err := r.Pool.Exec(ctx, query, newToken, roomID, propertyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("room %s not found for this property", roomID)
	}
	return nil
}

func (r *PostgresRepository) GetCatalogItems(ctx context.Context, propertyID, search string) ([]model.CatalogItem, error) {
	query := `
		SELECT id, property_id, service_type, name, description, price, is_available, attributes, created_at
		FROM catalog_items
		WHERE property_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
		ORDER BY service_type, name`
	rows, err := r.Pool.Query(ctx, query, propertyID, search)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.CatalogItem
	for rows.Next() {
		var item model.CatalogItem
		err := rows.Scan(&item.ID, &item.PropertyID, &item.ServiceType, &item.Name, &item.Description, &item.Price, &item.IsAvailable, &item.Attributes, &item.CreatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PostgresRepository) GetCatalogItemByID(ctx context.Context, itemID string) (*model.CatalogItem, error) {
	query := `SELECT id, property_id, service_type, name, description, price, is_available, attributes, created_at FROM catalog_items WHERE id = $1`
	var item model.CatalogItem
	err := r.Pool.QueryRow(ctx, query, itemID).Scan(&item.ID, &item.PropertyID, &item.ServiceType, &item.Name, &item.Description, &item.Price, &item.IsAvailable, &item.Attributes, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *PostgresRepository) CreateCatalogItem(ctx context.Context, item *model.CatalogItem) error {
	query := `
		INSERT INTO catalog_items (id, property_id, service_type, name, description, price, is_available, attributes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`
	
	if item.ID == "" {
		item.ID = uuid.New().String()
	}

	return r.Pool.QueryRow(ctx, query, 
		item.ID, 
		item.PropertyID, 
		item.ServiceType, 
		item.Name, 
		item.Description, 
		item.Price, 
		item.IsAvailable, 
		item.Attributes,
	).Scan(&item.CreatedAt)
}

func (r *PostgresRepository) UpdateCatalogItem(ctx context.Context, item *model.CatalogItem) error {
	query := `
		UPDATE catalog_items
		SET name = $1, description = $2, price = $3, is_available = $4, attributes = $5
		WHERE id = $6 AND property_id = $7`
	
	_, err := r.Pool.Exec(ctx, query,
		item.Name,
		item.Description,
		item.Price,
		item.IsAvailable,
		item.Attributes,
		item.ID,
		item.PropertyID,
	)
	return err
}

func (r *PostgresRepository) DeleteCatalogItem(ctx context.Context, itemID, propertyID string) error {
	query := `DELETE FROM catalog_items WHERE id = $1 AND property_id = $2`
	_, err := r.Pool.Exec(ctx, query, itemID, propertyID)
	return err
}

func (r *PostgresRepository) CreateOrder(ctx context.Context, order *model.Order) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	order.ID = uuid.New().String()
	order.CreatedAt = time.Now()
	order.Status = model.StatusPending

	var sessionIDVal *string
	if order.SessionID != "" {
		sessionIDVal = &order.SessionID
	}

	orderQuery := `INSERT INTO orders (id, room_id, qr_token, session_id, group_id, status, total_amount, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err = tx.Exec(ctx, orderQuery, order.ID, order.RoomID, order.QRToken, sessionIDVal, order.GroupID, order.Status, order.TotalAmount, order.CreatedAt)
	if err != nil {
		return err
	}

	itemQuery := `INSERT INTO order_items (id, order_id, catalog_item_id, quantity, price, attributes) VALUES ($1, $2, $3, $4, $5, $6)`
	for i := range order.Items {
		item := &order.Items[i]
		item.ID = uuid.New().String()
		item.OrderID = order.ID

		_, err = tx.Exec(ctx, itemQuery, item.ID, item.OrderID, item.CatalogItemID, item.Quantity, item.Price, item.Attributes)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) GetOrdersByProperty(ctx context.Context, propertyID string) ([]model.Order, error) {
	orderQuery := `
		SELECT o.id, o.room_id, r.room_number, r.property_id, o.qr_token, COALESCE(o.session_id::text, ''), o.group_id, o.status, o.total_amount, o.created_at 
		FROM orders o
		JOIN rooms r ON o.room_id = r.id
		WHERE r.property_id = $1
		ORDER BY o.created_at DESC`
	
	rows, err := r.Pool.Query(ctx, orderQuery, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var o model.Order
		if err := rows.Scan(&o.ID, &o.RoomID, &o.RoomNumber, &o.PropertyID, &o.QRToken, &o.SessionID, &o.GroupID, &o.Status, &o.TotalAmount, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}

	for i := range orders {
		items, err := r.getOrderItems(ctx, orders[i].ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = items
	}

	return orders, nil
}

func (r *PostgresRepository) GetActiveOrdersByRoomToken(ctx context.Context, qrToken string) ([]model.Order, error) {
	orderQuery := `
		SELECT o.id, o.room_id, r.room_number, r.property_id, o.qr_token, COALESCE(o.session_id::text, ''), o.group_id, o.status, o.total_amount, o.created_at 
		FROM orders o
		JOIN rooms r ON o.room_id = r.id
		WHERE o.qr_token = $1 AND o.status IN ('pending', 'accepted')
		ORDER BY o.created_at DESC`
	
	rows, err := r.Pool.Query(ctx, orderQuery, qrToken)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var o model.Order
		if err := rows.Scan(&o.ID, &o.RoomID, &o.RoomNumber, &o.PropertyID, &o.QRToken, &o.SessionID, &o.GroupID, &o.Status, &o.TotalAmount, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}

	for i := range orders {
		items, err := r.getOrderItems(ctx, orders[i].ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = items
	}

	return orders, nil
}

func (r *PostgresRepository) GetOrdersByRoomToken(ctx context.Context, qrToken string) ([]model.Order, error) {
	orderQuery := `
		SELECT o.id, o.room_id, r.room_number, r.property_id, o.qr_token, COALESCE(o.session_id::text, ''), o.group_id, o.status, o.total_amount, o.created_at 
		FROM orders o
		JOIN rooms r ON o.room_id = r.id
		WHERE o.qr_token = $1
		ORDER BY o.created_at DESC`
	
	rows, err := r.Pool.Query(ctx, orderQuery, qrToken)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var o model.Order
		if err := rows.Scan(&o.ID, &o.RoomID, &o.RoomNumber, &o.PropertyID, &o.QRToken, &o.SessionID, &o.GroupID, &o.Status, &o.TotalAmount, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}

	for i := range orders {
		items, err := r.getOrderItems(ctx, orders[i].ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = items
	}

	return orders, nil
}

func (r *PostgresRepository) GetOrder(ctx context.Context, orderID string) (*model.Order, error) {
	query := `
		SELECT o.id, o.room_id, r.room_number, r.property_id, o.qr_token, COALESCE(o.session_id::text, ''), o.group_id, o.status, o.total_amount, o.created_at 
		FROM orders o
		JOIN rooms r ON o.room_id = r.id
		WHERE o.id = $1`
	
	var o model.Order
	err := r.Pool.QueryRow(ctx, query, orderID).Scan(&o.ID, &o.RoomID, &o.RoomNumber, &o.PropertyID, &o.QRToken, &o.SessionID, &o.GroupID, &o.Status, &o.TotalAmount, &o.CreatedAt)
	if err != nil {
		return nil, err
	}

	items, err := r.getOrderItems(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	o.Items = items

	return &o, nil
}

// --- Guest Session Management Methods ---

func (r *PostgresRepository) GetRoomByID(ctx context.Context, id string) (*model.Room, error) {
	query := `SELECT id, property_id, room_number, floor, building, qr_token, created_at FROM rooms WHERE id = $1`
	var rm model.Room
	err := r.Pool.QueryRow(ctx, query, id).Scan(&rm.ID, &rm.PropertyID, &rm.RoomNumber, &rm.Floor, &rm.Building, &rm.QRToken, &rm.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (r *PostgresRepository) CreateGuestSession(ctx context.Context, roomID, token string, expiresAt time.Time) (*model.GuestSession, error) {
	query := `
		INSERT INTO guest_sessions (id, room_id, session_token, status, expires_at)
		VALUES ($1, $2, $3, 'active', $4)
		RETURNING id, status, created_at`
	
	id := uuid.New().String()
	var s model.GuestSession
	s.ID = id
	s.RoomID = roomID
	s.SessionToken = token
	s.ExpiresAt = expiresAt
	
	err := r.Pool.QueryRow(ctx, query, id, roomID, token, expiresAt).Scan(&s.ID, &s.Status, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresRepository) GetGuestSessionByToken(ctx context.Context, token string) (*model.GuestSession, error) {
	query := `
		SELECT id, room_id, session_token, status, created_at, expires_at 
		FROM guest_sessions 
		WHERE session_token = $1`
	var s model.GuestSession
	err := r.Pool.QueryRow(ctx, query, token).Scan(&s.ID, &s.RoomID, &s.SessionToken, &s.Status, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresRepository) InvalidateAllGuestSessionsForRoom(ctx context.Context, roomID string) error {
	query := `UPDATE guest_sessions SET status = 'archived' WHERE room_id = $1 AND status = 'active'`
	_, err := r.Pool.Exec(ctx, query, roomID)
	return err
}

func (r *PostgresRepository) ExtendGuestSession(ctx context.Context, sessionID string, newExpiresAt time.Time) error {
	query := `UPDATE guest_sessions SET status = 'active', expires_at = $1 WHERE id = $2`
	_, err := r.Pool.Exec(ctx, query, newExpiresAt, sessionID)
	return err
}

func (r *PostgresRepository) UpdateGuestSessionStatus(ctx context.Context, sessionID string, status string) error {
	query := `UPDATE guest_sessions SET status = $1 WHERE id = $2`
	_, err := r.Pool.Exec(ctx, query, status, sessionID)
	return err
}

func (r *PostgresRepository) GetActiveGuestSessionForRoom(ctx context.Context, roomID string) (*model.GuestSession, error) {
	query := `
		SELECT id, room_id, session_token, status, created_at, expires_at
		FROM guest_sessions
		WHERE room_id = $1 AND status = 'active' AND expires_at > NOW()
		ORDER BY created_at DESC LIMIT 1`
	var s model.GuestSession
	err := r.Pool.QueryRow(ctx, query, roomID).Scan(&s.ID, &s.RoomID, &s.SessionToken, &s.Status, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresRepository) GetActiveOrdersBySessionID(ctx context.Context, sessionID string) ([]model.Order, error) {
	orderQuery := `
		SELECT o.id, o.room_id, r.room_number, r.property_id, o.qr_token, COALESCE(o.session_id::text, ''), o.group_id, o.status, o.total_amount, o.created_at 
		FROM orders o
		JOIN rooms r ON o.room_id = r.id
		WHERE o.session_id = $1 AND o.status IN ('pending', 'accepted')
		ORDER BY o.created_at DESC`
	
	rows, err := r.Pool.Query(ctx, orderQuery, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var o model.Order
		if err := rows.Scan(&o.ID, &o.RoomID, &o.RoomNumber, &o.PropertyID, &o.QRToken, &o.SessionID, &o.GroupID, &o.Status, &o.TotalAmount, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}

	for i := range orders {
		items, err := r.getOrderItems(ctx, orders[i].ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = items
	}

	return orders, nil
}

func (r *PostgresRepository) GetOrdersBySessionID(ctx context.Context, sessionID string) ([]model.Order, error) {
	orderQuery := `
		SELECT o.id, o.room_id, r.room_number, r.property_id, o.qr_token, COALESCE(o.session_id::text, ''), o.group_id, o.status, o.total_amount, o.created_at 
		FROM orders o
		JOIN rooms r ON o.room_id = r.id
		WHERE o.session_id = $1
		ORDER BY o.created_at DESC`
	
	rows, err := r.Pool.Query(ctx, orderQuery, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var o model.Order
		if err := rows.Scan(&o.ID, &o.RoomID, &o.RoomNumber, &o.PropertyID, &o.QRToken, &o.SessionID, &o.GroupID, &o.Status, &o.TotalAmount, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}

	for i := range orders {
		items, err := r.getOrderItems(ctx, orders[i].ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = items
	}

	return orders, nil
}


func (r *PostgresRepository) UpdateOrderStatus(ctx context.Context, orderID string, status model.OrderStatus) error {
	query := `UPDATE orders SET status = $1 WHERE id = $2`
	tag, err := r.Pool.Exec(ctx, query, status, orderID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("order %s not found", orderID)
	}
	return nil
}

func (r *PostgresRepository) RemoveOrderItem(ctx context.Context, orderID string, itemID string) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	deleteQuery := `DELETE FROM order_items WHERE id = $1 AND order_id = $2`
	tag, err := tx.Exec(ctx, deleteQuery, itemID, orderID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("item %s not found in order %s", itemID, orderID)
	}

	sumQuery := `SELECT COALESCE(SUM(price * quantity), 0) FROM order_items WHERE order_id = $1`
	var newTotal float64
	if err := tx.QueryRow(ctx, sumQuery, orderID).Scan(&newTotal); err != nil {
		return err
	}

	updateQuery := `UPDATE orders SET total_amount = $1 WHERE id = $2`
	if _, err := tx.Exec(ctx, updateQuery, newTotal, orderID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) getOrderItems(ctx context.Context, orderID string) ([]model.OrderItem, error) {
	query := `
		SELECT oi.id, oi.order_id, oi.catalog_item_id, ci.name, ci.service_type, oi.quantity, oi.price, oi.attributes 
		FROM order_items oi
		JOIN catalog_items ci ON oi.catalog_item_id = ci.id
		WHERE oi.order_id = $1`
	
	rows, err := r.Pool.Query(ctx, query, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.OrderItem
	for rows.Next() {
		var item model.OrderItem
		if err := rows.Scan(&item.ID, &item.OrderID, &item.CatalogItemID, &item.ItemName, &item.ServiceType, &item.Quantity, &item.Price, &item.Attributes); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
