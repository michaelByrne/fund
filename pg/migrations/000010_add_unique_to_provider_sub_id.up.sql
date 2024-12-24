UPDATE donation
SET provider_subscription_id = CONCAT('placeholder_', id) -- Or use a UUID
WHERE provider_subscription_id IS NULL
   OR provider_subscription_id = '';

ALTER TABLE donation
    ADD CONSTRAINT check_provider_subscription_id
        CHECK (
            donation_plan_id IS NULL OR provider_subscription_id IS NOT NULL
            );

ALTER TABLE donation
    ADD CONSTRAINT unique_provider_subscription_id UNIQUE (provider_subscription_id);
