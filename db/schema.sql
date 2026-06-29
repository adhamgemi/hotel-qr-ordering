-- Enable UUID generation support if needed (PostgreSQL 13+ has gen_random_uuid() built-in)
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE IF NOT EXISTS properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    property_id UUID NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS property_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    property_id UUID NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    service_type VARCHAR(50) NOT NULL, -- e.g., 'fnb', 'laundry', 'housekeeping', 'maintenance', 'concierge'
    is_enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(property_id, service_type)
);

CREATE TABLE IF NOT EXISTS rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    property_id UUID NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    room_number VARCHAR(50) NOT NULL,
    floor VARCHAR(50) DEFAULT '',
    building VARCHAR(100) DEFAULT '',
    qr_token VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS catalog_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    property_id UUID NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    service_type VARCHAR(50) NOT NULL, -- Ties the item to a specific module
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    is_available BOOLEAN DEFAULT TRUE,
    attributes JSONB, -- Flexible column for images, allergens, laundry types, etc.
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE RESTRICT,
    qr_token VARCHAR(255) DEFAULT '',
    group_id VARCHAR(255) DEFAULT '',
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- pending, accepted, completed, cancelled
    total_amount DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    catalog_item_id UUID NOT NULL REFERENCES catalog_items(id) ON DELETE RESTRICT,
    quantity INT NOT NULL CHECK (quantity > 0),
    price DECIMAL(10, 2) NOT NULL DEFAULT 0.00, -- Store price at time of order
    attributes JSONB -- Capture any specific selections made by the user
);

-- Indexes for performance
ALTER TABLE orders ADD COLUMN IF NOT EXISTS qr_token VARCHAR(255) DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS group_id VARCHAR(255) DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS session_id UUID DEFAULT NULL;

CREATE TABLE IF NOT EXISTS guest_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    session_token VARCHAR(255) UNIQUE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- active, archived
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_guest_sessions_token ON guest_sessions(session_token);
CREATE INDEX IF NOT EXISTS idx_guest_sessions_room_id ON guest_sessions(room_id);
CREATE INDEX IF NOT EXISTS idx_orders_session_id ON orders(session_id);

ALTER TABLE rooms ADD COLUMN IF NOT EXISTS floor VARCHAR(50) DEFAULT '';
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS building VARCHAR(100) DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_users_property_id ON users(property_id);
CREATE INDEX IF NOT EXISTS idx_property_services_property_id ON property_services(property_id);
CREATE INDEX IF NOT EXISTS idx_rooms_property_id ON rooms(property_id);
CREATE INDEX IF NOT EXISTS idx_catalog_items_property_id ON catalog_items(property_id);
CREATE INDEX IF NOT EXISTS idx_catalog_items_service_type ON catalog_items(service_type);
CREATE INDEX IF NOT EXISTS idx_orders_room_id ON orders(room_id);
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items(order_id);
CREATE INDEX IF NOT EXISTS idx_orders_qr_token ON orders(qr_token);

CREATE INDEX IF NOT EXISTS idx_rooms_trgm ON rooms USING gin (room_number gin_trgm_ops, floor gin_trgm_ops, building gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_catalog_trgm ON catalog_items USING gin (name gin_trgm_ops, description gin_trgm_ops);
