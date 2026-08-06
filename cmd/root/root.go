package root

import (
	"boardfund/aws"
	"boardfund/jwtauth"
	"boardfund/jwtauth/keyset"
	"boardfund/messaging"
	"boardfund/paypal"
	"boardfund/paypal/token"
	"boardfund/pg"
	"boardfund/service/auth"
	"boardfund/service/auth/store"
	"boardfund/service/donations"
	donationstore "boardfund/service/donations/store"
	"boardfund/service/enrollments"
	enrollmentstore "boardfund/service/enrollments/store"
	"boardfund/service/finance"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"
	"boardfund/service/members"
	memberstore "boardfund/service/members/store"
	"boardfund/service/payouts"
	payoutstore "boardfund/service/payouts/store"
	"boardfund/web/adminweb"
	"boardfund/web/authweb"
	"boardfund/web/common"
	"boardfund/web/homeweb"
	"boardfund/web/hooksweb"
	"boardfund/web/middlewares"
	"boardfund/web/mux"
	"context"
	"errors"
	"fmt"
	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/aws/aws-sdk-go-v2/config"
	cognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type PayPalConfig struct {
	ClientID     string
	ClientSecret string
	BaseURL      string
	WebhookID    string
	ProductID    string
}

type RunConfig struct {
	PayPal PayPalConfig
	IsLive bool

	Host string

	PGUser string
	PGPass string
	PGHost string
	PGPort string
	PGDB   string

	JWKURL            string
	CognitoClientID   string
	CognitoUserPoolID string

	EnableNATSLogging bool

	// NATSStoreDir is where JetStream keeps the webhook stream. Must be on a
	// mounted volume; see runNATS.
	NATSStoreDir                     string
	DonationsPaymentsReportsS3Bucket string

	ReportTypes []string

	// PayoutApprovalWindow is how long a treasurer has to approve a batch before it
	// is cancelled. PayoutReminderWindow is how close to that deadline a batch must
	// be before a reminder is sent. Both fall back to service defaults when zero.
	PayoutApprovalWindow time.Duration
	PayoutReminderWindow time.Duration
}

type ChildDeps struct {
	FinanceSvc *finance.FinanceService
}

func RootCmd(ctx context.Context, runConfig RunConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fund",
		Short: "bco mutual aid",

		// A failing command prints its error, not the whole usage block. These
		// run unattended on a schedule, where the reason a payout sweep failed
		// should not be buried under a flag listing.
		//
		// Cobra also prints the error itself by default, which main.go then
		// prints again via log.Fatal. Silencing that leaves one copy, timestamped.
		SilenceUsage:  true,
		SilenceErrors: true,

		RunE: func(cmd *cobra.Command, args []string) error {
			return run(ctx, runConfig)
		},
	}

	return cmd
}

func run(ctx context.Context, runConfig RunConfig) error {
	jsonHandler := slog.NewJSONHandler(os.Stdout, nil)
	logger := slog.New(jsonHandler)

	storeDir := runConfig.NATSStoreDir
	if storeDir == "" {
		// Not fatal, because a webhook bus that will not start is worse than one
		// that is not durable. Logged at error so it cannot pass for normal:
		// without a mounted volume this is the old behaviour with extra steps.
		storeDir = filepath.Join(os.TempDir(), "fund-jetstream")

		logger.Error("NATS_STORE_DIR is not set, so webhook events will not survive a deploy",
			slog.String("falling_back_to", storeDir),
		)
	}

	nc, ns, err := runNATS(runConfig.EnableNATSLogging, storeDir)
	if err != nil {
		return err
	}
	defer nc.Close()

	dbURI := fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s",
		runConfig.PGUser, runConfig.PGPass, runConfig.PGHost, runConfig.PGPort, runConfig.PGDB,
	)

	tokenClient := token.NewClient(
		runConfig.PayPal.ClientID,
		runConfig.PayPal.ClientSecret,
		runConfig.PayPal.BaseURL,
	)
	tokenStore := token.NewStore(tokenClient)
	paypalClient := paypal.NewClient(tokenStore, logger, runConfig.PayPal.BaseURL)
	paypalService := paypal.NewPaypal(paypalClient, runConfig.PayPal.ProductID)

	pool, err := pg.GetDBPool(dbURI)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)

	fs := os.DirFS("pg/migrations")
	d, err := iofs.New(fs, ".")
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
	enrollmentStore := enrollmentstore.NewEnrollmentStore(pool)
	payoutStore := payoutstore.NewPayoutStore(pool)
	eventStore := fundeventstore.NewEventStore(pool)
	authStore := store.NewAuthStore(pool)
	sessionManager := scs.New()
	sessionManager.IdleTimeout = 1 * time.Hour
	sessionManager.Lifetime = 2 * time.Hour

	sessionManager.Store = pgxstore.New(pool)
	defaultConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-west-2"))
	if err != nil {
		return err
	}
	s3Client := s3.NewFromConfig(defaultConfig)
	cognitoClient := cognito.NewFromConfig(defaultConfig)

	authorizer := aws.NewCognitoAuth(cognitoClient, logger, runConfig.CognitoClientID, runConfig.CognitoUserPoolID)

	documentStorage := aws.NewAWSS3(s3Client, logger, "")

	ksetCache := keyset.NewKeySetWithCache(runConfig.JWKURL, 15)
	kset, err := ksetCache.NewKeySet()
	if err != nil {
		return err
	}
	verifier := jwtauth.NewToken(kset)

	messageBroker, err := messaging.NewBroker(ctx, nc, logger)
	if err != nil {
		return err
	}
	defer messageBroker.Close()

	fundEvents := fundevents.NewService(eventStore, logger)

	donationService := donations.NewDonationService(donationStore, documentStorage, paypalService, fundEvents, runConfig.ReportTypes, logger)
	memberService := members.NewMemberService(memberStore, donationStore, paypalService, fundEvents, logger)
	authService := auth.NewAuthService(memberStore, authStore, authorizer, logger)
	financeService := finance.NewFinanceService(donationStore, paypalService, documentStorage, fundEvents, runConfig.ReportTypes, logger)
	enrollmentService := enrollments.NewEnrollmentsService(enrollmentStore, donationStore, fundEvents, logger)

	// No notifier yet: approval reminders are logged by the sweep until a delivery
	// channel exists. The service tolerates a nil notifier.
	payoutService := payouts.NewPayoutService(
		payoutStore, paypalService, nil, fundEvents,
		runConfig.PayoutApprovalWindow, runConfig.PayoutReminderWindow, logger,
	)

	authMiddleware := middlewares.Verify(
		verifier.Verify,
		middlewares.TokenFromCookie,
		middlewares.TokenFromHeader,
	)
	adminAuthMiddleware := middlewares.Verify(
		verifier.VerifyAdmin,
		middlewares.TokenFromCookie,
		middlewares.TokenFromHeader,
	)

	// Handlers setup
	donationHandlers := homeweb.NewFundHandlers(
		donationService, sessionManager, authMiddleware, logger,
		runConfig.PayPal.ProductID, runConfig.PayPal.ClientID,
	)
	authHandlers := authweb.NewAuthHandlers(authService, memberService, sessionManager, runConfig.PayPal.ClientID)
	adminHandlers := adminweb.NewAdminHandlers(
		adminAuthMiddleware, memberService, donationService, authService, financeService, enrollmentService, payoutService, fundEvents, sessionManager, runConfig.PayPal.ClientID,
	)
	webhooksHandlers := hooksweb.NewWebhooksHandlers(
		donationService, memberService, messageBroker, logger, runConfig.PayPal.WebhookID,
	)

	donationWebhookHandlers := donations.NewHandlers(donationStore, fundEvents, logger)
	err = donationWebhookHandlers.Subscribe(messageBroker)
	if err != nil {
		return err
	}

	payoutWebhookHandlers := payouts.NewHandlers(payoutStore, logger)
	err = payoutWebhookHandlers.Subscribe(messageBroker)
	if err != nil {
		return err
	}

	router := mux.NewRouter(http.NewServeMux())
	router.Use(sessionManager.LoadAndSave)

	// Assets are linked by a URL containing a hash of their contents, so a changed
	// file is a new URL. Asking politely for revalidation was not enough: the
	// origin sends no-cache and Cloudflare rewrites it to a four-hour max-age, so
	// the browser was handed a stylesheet older than the markup that needed it.
	// A cached copy of a hashed URL can only be of the bytes that URL names, which
	// makes any TTL correct rather than something to argue with.
	if err = common.LoadAssets("public"); err != nil {
		return err
	}

	staticFiles := http.StripPrefix("/static/", http.FileServer(http.Dir("public")))
	router.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		requested := strings.TrimPrefix(r.URL.Path, "/static/")

		file, hashed := common.ResolveAsset(requested)
		if hashed {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			// Serve the file the hash names. Rewriting the path rather than
			// redirecting keeps it one request.
			r.URL.Path = "/static/" + file
		} else {
			// An unhashed URL still means whatever the file holds today, so it has
			// to be revalidated. no-cache is "ask first", not "do not store".
			w.Header().Set("Cache-Control", "no-cache")
		}

		staticFiles.ServeHTTP(w, r)
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

		shutdownCtx, cancel := context.WithTimeout(serverCtx, 30*time.Second)
		defer cancel()

		go func() {
			<-shutdownCtx.Done()
			if errors.Is(shutdownCtx.Err(), context.DeadlineExceeded) {
				logger.Error("graceful shutdown timed out.. forcing exit.")
			}
			ns.Shutdown()
		}()

		err := server.Shutdown(shutdownCtx)
		if err != nil {
			logger.Error("server shutdown error", slog.String("error", err.Error()))
		}

		ns.WaitForShutdown()
		serverStopCtx()
	}()

	log.Println("** starting bco mutual aid on port 8080 **")
	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server failed with error: %w", err)
	}

	<-serverCtx.Done()

	return nil
}

// runNATS starts the embedded server with JetStream on disk.
//
// storeDir must be on a mounted volume. Railway's container filesystem is
// replaced on every deploy, so JetStream pointed anywhere else is durable across
// a process restart and nothing else -- which is the failure mode this change
// exists to remove, wearing a persistence badge.
//
// DontListen stays: the server has never opened a port, and nothing outside this
// process connects to it.
func runNATS(enableLogging bool, storeDir string) (*nats.Conn, *server.Server, error) {
	opts := server.Options{
		DontListen: true,
		JetStream:  true,
		StoreDir:   storeDir,
	}

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
