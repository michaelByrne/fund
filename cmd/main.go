package main

import (
	"boardfund/cmd/root"
	"boardfund/cmd/root/audit"
	donationsaudit "boardfund/cmd/root/audit/donations"
	"context"
	"log"
	"os"
	"strconv"
	"strings"
)

func loadRunConfig() (*root.RunConfig, error) {
	// Determine if we are in production
	isLive := os.Getenv("IS_PROD") == "true"

	// Select PayPal config based on environment
	payPalConfig := root.PayPalConfig{
		ClientID:     getEnvOrError("DEV_PAYPAL_CLIENT_ID", !isLive),
		ClientSecret: getEnvOrError("DEV_PAYPAL_CLIENT_SECRET", !isLive),
		BaseURL:      getEnvOrError("DEV_PAYPAL_BASE_URL", !isLive),
		WebhookID:    getEnvOrError("DEV_PAYPAL_WEBHOOK_ID", !isLive),
		ProductID:    getEnvOrError("DEV_PAYPAL_PRODUCT_ID", !isLive),
	}

	if isLive {
		payPalConfig = root.PayPalConfig{
			ClientID:     getEnvOrError("PROD_PAYPAL_CLIENT_ID", isLive),
			ClientSecret: getEnvOrError("PROD_PAYPAL_CLIENT_SECRET", isLive),
			BaseURL:      getEnvOrError("PROD_PAYPAL_URL", isLive),
			WebhookID:    getEnvOrError("PROD_PAYPAL_WEBHOOK_ID", isLive),
			ProductID:    getEnvOrError("PROD_PAYPAL_PRODUCT_ID", isLive),
		}
	}

	// General configurations
	config := &root.RunConfig{
		PayPal: payPalConfig,
		IsLive: isLive,

		PGUser: getEnvOrError("PG_USER", true),
		PGPass: getEnvOrError("PG_PASS", true),
		PGHost: getEnvOrError("PG_HOST", true),
		PGPort: getEnvOrDefault("PG_PORT", "5432"),
		PGDB:   getEnvOrError("PG_DB", true),
		Host:   getEnvOrDefault("HOST", "localhost"),

		JWKURL:            getEnvOrError("JWK_URL", true),
		CognitoClientID:   getEnvOrError("COGNITO_CLIENT_ID", true),
		CognitoUserPoolID: getEnvOrError("COGNITO_USER_POOL_ID", true),

		EnableNATSLogging: getEnvAsBool("ENABLE_NATS_LOGGING", false),
		ReportTypes:       getEnvAsSlice("ENABLED_REPORT_TYPES"),
	}

	return config, nil
}

func getEnvOrError(key string, required bool) string {
	value, exists := os.LookupEnv(key)
	if required && !exists {
		log.Fatal(key + " is required")
	}
	return value
}

func getEnvOrDefault(key string, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsSlice(key string) []string {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return []string{}
	}

	return strings.Split(valueStr, ",")
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func main() {
	ctx := context.Background()

	runConfig, err := loadRunConfig()
	if err != nil {
		log.Fatal(err)
	}

	rootCmd := root.RootCmd(ctx, *runConfig)
	auditCmd := audit.AuditCmd()
	donationsAuditCmd := donationsaudit.DonationsAuditCmd(runConfig)

	auditCmd.AddCommand(donationsAuditCmd)
	rootCmd.AddCommand(auditCmd)

	rootCmd.Execute()
}
