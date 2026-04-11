package main

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"time"

	config "be-modami-chat-service/config"
	_ "be-modami-chat-service/docs" // Swagger generated docs
	chatcache "be-modami-chat-service/internal/adapter/cache"
	chathandler "be-modami-chat-service/internal/adapter/handler"
	"be-modami-chat-service/internal/adapter/handler/middleware"
	repository "be-modami-chat-service/internal/adapter/repository"
	"be-modami-chat-service/internal/service"
	pkgkafka "be-modami-chat-service/pkg/kafka"
	"be-modami-chat-service/pkg/kafka/events"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	httpswagger "github.com/swaggo/http-swagger"

	logging "gitlab.com/lifegoeson-libs/pkg-logging"
	"gitlab.com/lifegoeson-libs/pkg-logging/logger"
	pkgloggingmw "gitlab.com/lifegoeson-libs/pkg-logging/middleware"
)

type Application struct {
	HTTPServer    *http.Server
	KafkaService  *pkgkafka.KafkaService
	ChatConsumer  *pkgkafka.Consumer
	PresenceStore *chatcache.PresenceStore
}

func NewApplication(ctx context.Context, cfg *config.Config, conn *Connections) *Application {
	// Repositories & adapters
	msgRepo := repository.NewMessageRepo(conn.ScyllaSession)
	convRepo := repository.NewConversationRepo(conn.ScyllaSession)
	cacheStore := chatcache.NewCacheStore(conn.RedisClient)
	presenceStore := chatcache.NewPresenceStore(conn.RedisClient)
	rateLimiter := chatcache.NewRateLimiter(conn.RedisClient)

	// Service
	chatService := service.NewChatService(
		msgRepo,
		convRepo,
		conn.KafkaProducer,
		conn.CentrifugoPublisher,
		cacheStore,
		presenceStore,
		rateLimiter,
		conn.IDGen,
	)

	// Auth middleware (Keycloak JWKS)
	authMW, err := middleware.NewAuthMiddleware(cfg.Keycloak.JWKSUrl)
	if err != nil {
		// Non-fatal: keys will be refreshed lazily on first request.
		logger.Warn(ctx, "jwks initial fetch failed, will retry on first request", logging.String("error", err.Error()))
	}

	// HTTP server
	chatHandler := chathandler.NewHandler(chatService)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.App.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           cfg.App.CORSMaxAge,
	}))

	// Request body size limit (1MB)
	r.Use(func(next http.Handler) http.Handler {
		return http.MaxBytesHandler(next, 1<<20)
	})

	// Health check (no auth)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Swagger UI — available at /swagger/index.html
	r.Get("/swagger/*", httpswagger.Handler(
		httpswagger.URL("/swagger/doc.json"),
	))

	// Centrifugo proxy endpoints (called by Centrifugo internally)
	centrifugoProxy := chathandler.NewCentrifugoProxy(authMW, chatService)
	r.Route("/centrifugo/proxy", func(r chi.Router) {
		r.Post("/connect", centrifugoProxy.HandleConnect)
		r.Post("/subscribe", centrifugoProxy.HandleSubscribe)
	})

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.JWTAuth(authMW))
		r.Route("/api/v1", func(r chi.Router) {
			chatHandler.RegisterRoutes(r)
		})
	})

	addr := cfg.App.ListenAddr()

	// Wrap the router with OTel tracing + metrics middleware.
	handler := pkgloggingmw.HTTPMiddleware("chat-service", r, &pkgloggingmw.HttpLoggingOptions{
		ExceptRoutes: []string{"/health"},
	})

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  cfg.App.GetReadTimeout(),
		WriteTimeout: cfg.App.GetWriteTimeout(),
	}

	// Kafka consumer — handles inbound messages.
	chatConsumer := pkgkafka.NewConsumer("chat-inbound", conn.KafkaService)
	chatConsumer.RegisterHandler(pkgkafka.NewTopicHandler(
		pkgkafka.TopicMessagesInbound,
		func(ctx context.Context, payload any) error {
			event, ok := payload.(events.KafkaEventBase)
			if !ok {
				return nil
			}
			logger.Debug(ctx, "processing inbound kafka message",
				logging.String("event_type", event.EventType),
				logging.String("event_id", event.EventID),
			)
			return nil
		},
		reflect.TypeOf(events.KafkaEventBase{}),
		nil,
	))

	return &Application{
		HTTPServer:    srv,
		KafkaService:  conn.KafkaService,
		ChatConsumer:  chatConsumer,
		PresenceStore: presenceStore,
	}
}

func (a *Application) Start(ctx context.Context) {
	// Start Kafka consumer
	go func() {
		if err := a.KafkaService.StartConsumer(ctx, []pkgkafka.ConsumerHandler{a.ChatConsumer}); err != nil && ctx.Err() == nil {
			logger.Error(ctx, "kafka consumer error", err)
		}
	}()

	// Start presence cleanup goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := a.PresenceStore.CleanupStalePresence(ctx); err != nil {
					logger.Error(ctx, "presence cleanup error", err)
				}
			}
		}
	}()

	// Start HTTP server
	go func() {
		logger.Info(ctx, "http server started", logging.String("addr", a.HTTPServer.Addr))
		if err := a.HTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "http server error", err)
			os.Exit(1)
		}
	}()
}

func (a *Application) Shutdown(ctx context.Context) {
	if err := a.HTTPServer.Shutdown(ctx); err != nil {
		logger.Error(ctx, "http server shutdown error", err)
	}
	a.KafkaService.Close()
}
