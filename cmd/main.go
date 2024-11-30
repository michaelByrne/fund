package main

import (
	"boardfund/paypal"
	"boardfund/paypal/token"
	"boardfund/service/donations"
	donationstore "boardfund/service/donations/store"
	memberstore "boardfund/service/members/store"
	"boardfund/web"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

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

	dbURI := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s", pgUser, pgPass, pgHost, pgPort, pgDB)

	tokenClient := token.NewClient(clientID, clientSecret, baseURL)
	tokenStore := token.NewStore(tokenClient)
	paypalClient := paypal.NewClient(tokenStore, baseURL)
	paypalService := paypal.NewPaypal(paypalClient, productID)

	pool, err := pgxpool.Connect(ctx, dbURI)
	if err != nil {
		return fmt.Errorf("failed to create a db connection pool: %w", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire a db connection: %w", err)
	}

	defer conn.Release()

	donationStore := donationstore.NewDonationStore(conn)
	memberStore := memberstore.NewMemberStore(conn)

	jsonHandler := slog.NewJSONHandler(stdout, nil)
	logger := slog.New(jsonHandler)

	donationService := donations.NewDonationService(donationStore, memberStore, paypalService, logger)
	donationHandler := web.NewDonationHandler(donationService, productID, clientID)

	router := http.NewServeMux()

	donationHandler.Register(router)

	router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))

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
