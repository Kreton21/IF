CREATE TABLE IF NOT EXISTS bus_stations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bus_departures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    station_id UUID NOT NULL REFERENCES bus_stations(id) ON DELETE CASCADE,
    direction TEXT NOT NULL CHECK (direction IN ('to_festival', 'from_festival')),
    departure_time TIMESTAMPTZ NOT NULL,
    price_cents INT NOT NULL DEFAULT 400 CHECK (price_cents >= 0),
    capacity INT NOT NULL CHECK (capacity > 0),
    sold INT NOT NULL DEFAULT 0 CHECK (sold >= 0),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bus_departures_station_direction_time
    ON bus_departures(station_id, direction, departure_time);

CREATE TABLE IF NOT EXISTS bus_order_rides (
    id BIGSERIAL PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    departure_id UUID NOT NULL REFERENCES bus_departures(id),
    ride_kind TEXT NOT NULL CHECK (ride_kind IN ('outbound', 'return')),
    from_station TEXT NOT NULL,
    to_station TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bus_order_rides_order ON bus_order_rides(order_id);

CREATE TABLE IF NOT EXISTS bus_tickets (
    ticket_id UUID PRIMARY KEY REFERENCES tickets(id) ON DELETE CASCADE,
    outbound_departure_id UUID NOT NULL REFERENCES bus_departures(id),
    return_departure_id UUID REFERENCES bus_departures(id),
    from_station TEXT NOT NULL,
    to_station TEXT NOT NULL,
    is_round_trip BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bus_tickets_outbound ON bus_tickets(outbound_departure_id);

INSERT INTO bus_stations (name)
VALUES
    ('Massy Palaiseau'),
    ('Orsay Ville'),
    ('Gif-sur-Yvette')
ON CONFLICT (name) DO NOTHING;
