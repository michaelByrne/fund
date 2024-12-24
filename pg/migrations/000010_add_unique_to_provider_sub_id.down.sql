ALTER TABLE donation
    DROP CONSTRAINT check_provider_subscription_id;

ALTER TABLE donation
    ALTER COLUMN provider_subscription_id DROP NOT NULL;

UPDATE donation
SET provider_subscription_id = NULL
WHERE provider_subscription_id LIKE 'placeholder_%';
