-- Keep bus_departures.sold synchronized with bus_order_rides
CREATE OR REPLACE FUNCTION sync_bus_departure_sold()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE bus_departures
        SET sold = (SELECT COUNT(*) FROM bus_order_rides WHERE departure_id = NEW.departure_id)
        WHERE id = NEW.departure_id;
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE bus_departures
        SET sold = (SELECT COUNT(*) FROM bus_order_rides WHERE departure_id = OLD.departure_id)
        WHERE id = OLD.departure_id;
        RETURN OLD;
    ELSIF TG_OP = 'UPDATE' THEN
        IF NEW.departure_id <> OLD.departure_id THEN
            UPDATE bus_departures
            SET sold = (SELECT COUNT(*) FROM bus_order_rides WHERE departure_id = OLD.departure_id)
            WHERE id = OLD.departure_id;
            UPDATE bus_departures
            SET sold = (SELECT COUNT(*) FROM bus_order_rides WHERE departure_id = NEW.departure_id)
            WHERE id = NEW.departure_id;
        ELSE
            UPDATE bus_departures
            SET sold = (SELECT COUNT(*) FROM bus_order_rides WHERE departure_id = NEW.departure_id)
            WHERE id = NEW.departure_id;
        END IF;
        RETURN NEW;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_bus_departure_sold ON bus_order_rides;
CREATE TRIGGER trg_sync_bus_departure_sold
AFTER INSERT OR UPDATE OR DELETE ON bus_order_rides
FOR EACH ROW EXECUTE FUNCTION sync_bus_departure_sold();
