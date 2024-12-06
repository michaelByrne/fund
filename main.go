package main

import (
	"boardfund/paypal"
	"boardfund/paypal/token"
	"boardfund/service/donations"
	donationstore "boardfund/service/donations/store"
	memberstore "boardfund/service/members/store"
	"boardfund/web/fundweb"
	"context"
	"embed"
	"errors"
	"fmt"
	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed pg/migrations/*.sql
var fs embed.FS

func main() {
	ctx := context.Background()

	err := run(ctx, os.Getenv, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, getEnv func(string) string, stdout io.Writer) error {
	clientID := getEnv("PAYPAL_CLIENT_ID")
	if clientID == "" {
		return fmt.Errorf("PAYPAL_CLIENT_ID is required")
	}

	clientSecret := getEnv("PAYPAL_CLIENT_SECRET")
	if clientSecret == "" {
		return fmt.Errorf("PAYPAL_CLIENT_SECRET is required")
	}

	baseURL := getEnv("PAYPAL_BASE_URL")
	if baseURL == "" {
		return fmt.Errorf("PAYPAL_BASE_URL is required")
	}

	productID := getEnv("PAYPAL_PRODUCT_ID")
	if productID == "" {
		return fmt.Errorf("PAYPAL_PRODUCT_ID is required")
	}

	pgPass := getEnv("PG_PASS")
	if pgPass == "" {
		return fmt.Errorf("PG_PASS is required")
	}

	pgUser := getEnv("PG_USER")
	if pgUser == "" {
		return fmt.Errorf("PG_USER is required")
	}

	pgHost := getEnv("PG_HOST")
	if pgHost == "" {
		return fmt.Errorf("PG_HOST is required")
	}

	pgPort := getEnv("PG_PORT")
	if pgPort == "" {
		return fmt.Errorf("PG_PORT is required")
	}

	pgDB := getEnv("PG_DB")
	if pgDB == "" {
		return fmt.Errorf("PG_DB is required")
	}

	jwkURL := getEnv("JWK_URL")
	if jwkURL == "" {
		return fmt.Errorf("JWK_URL is required")
	}

	dbURI := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s", pgUser, pgPass, pgHost, pgPort, pgDB)

	tokenClient := token.NewClient(clientID, clientSecret, baseURL)
	tokenStore := token.NewStore(tokenClient)
	paypalClient := paypal.NewClient(tokenStore, baseURL)
	paypalService := paypal.NewPaypal(paypalClient, productID)

	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)

	d, err := iofs.New(fs, "pg/migrations")
	if err != nil {
		return err
	}

	driver, err := pgx.WithInstance(db, &pgx.Config{})
	if err != nil {
		return err
	}

	migrator, err := migrate.NewWithInstance("iofs", d, "railway", driver)
	if err != nil {
		return err
	}

	err = migrator.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	donationStore := donationstore.NewDonationStore(pool)
	memberStore := memberstore.NewMemberStore(pool)
	sessionManager := scs.New()
	sessionManager.Store = pgxstore.New(pool)

	jsonHandler := slog.NewJSONHandler(stdout, nil)
	logger := slog.New(jsonHandler)

	//defaultConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-west-2"))
	//if err != nil {
	//	return err
	//}

	//cognitoClient := cognito.NewFromConfig(defaultConfig)

	//ksetCache := keyset.NewKeySetWithCache(jwkURL, 15)
	//kset, err := ksetCache.NewKeySet()
	//if err != nil {
	//	return err
	//}

	//verifier := jwtauth.NewToken(kset)

	//authMiddleware := middlewares.Verify(verifier.Verify, middlewares.TokenFromCookie, middlewares.TokenFromHeader)

	//authorizer := aws.NewCognitoAuth(cognitoClient, clientID)

	donationService := donations.NewDonationService(donationStore, memberStore, paypalService, logger)
	//authService := auth.NewAuthService(authorizer, logger)

	donationHandler := fundweb.NewFundHandler(donationService, sessionManager, productID, clientID)
	//authHandler := authweb.NewAuthHandler(authService, clientID)

	router := http.NewServeMux()

	router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))

	//authHandler.Register(router)
	donationHandler.Register(router)

	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	serverCtx, serverStopCtx := context.WithCancel(ctx)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		<-sig

		shutdownCtx, _ := context.WithTimeout(serverCtx, 30*time.Second)

		go func() {
			<-shutdownCtx.Done()
			if errors.Is(shutdownCtx.Err(), context.DeadlineExceeded) {
				log.Fatal("graceful shutdown timed out.. forcing exit.")
			}
		}()

		err := server.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal(err)
		}
		serverStopCtx()
	}()

	log.Println("** starting bco fund on port 8080 **")
	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server failed with error: %w", err)
	}

	<-serverCtx.Done()

	return nil
}
