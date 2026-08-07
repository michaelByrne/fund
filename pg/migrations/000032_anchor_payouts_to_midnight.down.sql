-- The trigger goes back to anchoring at the moment of creation. The times of day
-- truncated out of existing rows are not recoverable and are not restored.
CREATE OR REPLACE FUNCTION set_expires_default()
    RETURNS TRIGGER AS
$$
BEGIN
    IF NEW.payout_frequency = 'once' AND NEW.expires IS NULL THEN
        NEW.next_payment := now() + INTERVAL '1 month';
        NEW.expires := now() + INTERVAL '1 month';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
