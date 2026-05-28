-- Allow forcing bus departures to sold out
ALTER TABLE bus_departures
    ADD COLUMN IF NOT EXISTS is_sold_out BOOLEAN NOT NULL DEFAULT false;
