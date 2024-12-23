package main

import (
	"boardfund/aws"
	"boardfund/events"
	"boardfund/jwtauth"
	"boardfund/jwtauth/keyset"
	"boardfund/paypal"
	"boardfund/paypal/token"
	"boardfund/pg"
	"boardfund/service/auth"
	"boardfund/service/donations"
	donationstore "boardfund/service/donations/store"
	"boardfund/service/members"
	memberstore "boardfund/service/members/store"
	"boardfund/web/adminweb"
	"boardfund/web/authweb"
	"boardfund/web/homeweb"
	"boardfund/web/hooksweb"
	"boardfund/web/middlewares"
	"boardfund/web/mux"
	"context"
	"embed"
	"errors"
	"fmt"
	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/aws/aws-sdk-go-v2/config"
	cognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

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
	paypalClientID := getEnv("PAYPAL_CLIENT_ID")
	if paypalClientID == "" {
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

	webhookID := getEnv("SANDBOX_WEBHOOK_ID")
	if webhookID == "" {
		return fmt.Errorf("SANDBOX_WEBHOOK_ID is required")
	}

	productID := getEnv("PAYPAL_PRODUCT_ID")
	if productID == "" {
		return fmt.Errorf("PAYPAL_PRODUCT_ID is required")
	}

	isLive := getEnv("LIVE_PAYPAL")
	if isLive == "true" {
		baseURL = getEnv("LIVE_PAYPAL_URL")
		if baseURL == "" {
			return fmt.Errorf("LIVE_PAYPAL_URL is required")
		}

		paypalClientID = getEnv("LIVE_PAYPAL_CLIENT_ID")
		if paypalClientID == "" {
			return fmt.Errorf("LIVE_PAYPAL_CLIENT_ID is required")
		}

		clientSecret = getEnv("LIVE_PAYPAL_CLIENT_SECRET")
		if clientSecret == "" {
			return fmt.Errorf("LIVE_PAYPAL_CLIENT_SECRET is required")
		}

		webhookID = getEnv("LIVE_WEBHOOK_ID")
		if webhookID == "" {
			return fmt.Errorf("LIVE_WEBHOOK_ID is required")
		}

		productID = getEnv("LIVE_PAYPAL_PRODUCT_ID")
		if productID == "" {
			return fmt.Errorf("LIVE_PAYPAL_PRODUCT_ID is required")
		}
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

	cognitoClientID := getEnv("COGNITO_CLIENT_ID")
	if cognitoClientID == "" {
		return fmt.Errorf("COGNITO_CLIENT_ID is required")
	}

	userPoolID := getEnv("COGNITO_USER_POOL_ID")
	if userPoolID == "" {
		return fmt.Errorf("COGNITO_USER_POOL_ID is required")
	}

	var enableNATSLogging bool
	enableNATSLoggingStr := getEnv("ENABLE_NATS_LOGGING")
	if enableNATSLoggingStr == "true" {
		enableNATSLogging = true
	}

	nc, ns, err := runNATS(enableNATSLogging)
	if err != nil {
		return err
	}

	defer nc.Close()

	dbURI := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s", pgUser, pgPass, pgHost, pgPort, pgDB)

	tokenClient := token.NewClient(paypalClientID, clientSecret, baseURL)
	tokenStore := token.NewStore(tokenClient)
	paypalClient := paypal.NewClient(tokenStore, baseURL)
	paypalService := paypal.NewPaypal(paypalClient, productID)

	pool, err := pg.GetDBPool(dbURI)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)

	d, err := iofs.New(fs, "pg/migrations")
	if err != nil {
		return err
	}

	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
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

	defaultConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-west-2"))
	if err != nil {
		return err
	}

	cognitoClient := cognito.NewFromConfig(defaultConfig)

	ksetCache := keyset.NewKeySetWithCache(jwkURL, 15)
	kset, err := ksetCache.NewKeySet()
	if err != nil {
		return err
	}

	verifier := jwtauth.NewToken(kset)

	messageBroker := events.NewNATSMessageBroker(nc)

	authMiddleware := middlewares.Verify(verifier.Verify, middlewares.TokenFromCookie, middlewares.TokenFromHeader)
	adminAuthMiddleware := middlewares.Verify(verifier.VerifyAdmin, middlewares.TokenFromCookie, middlewares.TokenFromHeader)

	authorizer := aws.NewCognitoAuth(cognitoClient, logger, cognitoClientID, userPoolID)

	donationService := donations.NewDonationService(donationStore, paypalService, logger)
	memberService := members.NewMemberService(memberStore, donationStore, authorizer, paypalService, logger)
	authService := auth.NewAuthService(authorizer, memberStore, logger)

	donationHandlers := homeweb.NewFundHandlers(donationService, sessionManager, authMiddleware, logger, productID, paypalClientID)
	authHandlers := authweb.NewAuthHandlers(authService, sessionManager, paypalClientID)
	adminHandlers := adminweb.NewAdminHandlers(adminAuthMiddleware, memberService, donationService, sessionManager, paypalClientID)
	webhooksHandlers := hooksweb.NewWebhooksHandlers(donationService, memberService, &messageBroker, logger, webhookID)

	donationsEventHandlers := donations.NewHandlers(donationStore, logger)

	err = donationsEventHandlers.Subscribe(&messageBroker)
	if err != nil {
		return err
	}

	router := mux.NewRouter(http.NewServeMux())

	router.Use(sessionManager.LoadAndSave)

	router.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/static/", http.FileServer(http.Dir("public"))).ServeHTTP(w, r)
	})

	authHandlers.Register(router)
	donationHandlers.Register(router)
	adminHandlers.Register(router)
	webhooksHandlers.Register(router)

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

			ns.Shutdown()
		}()

		err = server.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal(err)
		}

		ns.WaitForShutdown()

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

func runNATS(enableLogging bool) (*nats.Conn, *server.Server, error) {
	opts := server.Options{DontListen: true}

	ns, err := server.NewServer(&opts)
	if err != nil {
		return nil, nil, err
	}

	if enableLogging {
		ns.ConfigureLogger()
	}

	go ns.Start()

	if !ns.ReadyForConnections(time.Second * 5) {
		return nil, nil, errors.New("nats server not ready")
	}

	clientOpts := []nats.Option{nats.InProcessServer(ns)}
	nc, err := nats.Connect(nats.DefaultURL, clientOpts...)
	if err != nil {
		return nil, nil, err
	}

	return nc, ns, nil
}
