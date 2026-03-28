package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	chathttp "be-modami-chat-service/internal/delivery/http"
	"be-modami-chat-service/internal/delivery/http/middleware"
	chatkafka "be-modami-chat-service/internal/kafka"
	"be-modami-chat-service/internal/service"
	mongorepo "be-modami-chat-service/internal/store/mongo"
	redisstore "be-modami-chat-service/internal/store/redis"
	"be-modami-chat-service/configs"
	"be-modami-chat-service/pkg/jwt"
	"be-modami-chat-service/pkg/kafka/events"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog/log"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Application struct {
	HTTPServer      *http.Server
	MessageConsumer *chatkafka.Consumer
	PresenceStore   *redisstore.PresenceStore
}

func NewApplication(ctx context.Context, cfg *config.Config, conn *Connections) *Application {
	// Ensure indexes
	if err := mongorepo.EnsureIndexes(ctx, conn.MongoDB); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure indexes")
	}

	// Repositories & adapters
	msgRepo := mongorepo.NewMessageRepo(conn.MongoDB)
	convRepo := mongorepo.NewConversationRepo(conn.MongoDB)
	cacheStore := redisstore.NewCacheStore(conn.RedisClient)
	presenceStore := redisstore.NewPresenceStore(conn.RedisClient)
	rateLimiter := redisstore.NewRateLimiter(conn.RedisClient)

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

	// HTTP server
	jwtService := jwt.NewService(cfg.JWT.Secret, cfg.JWT.Expiration)
	chatHandler := chathttp.NewHandler(chatService)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestLogger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORS.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           cfg.CORS.MaxAge,
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

	// Centrifugo proxy endpoints (called by Centrifugo internally)
	centrifugoProxy := chathttp.NewCentrifugoProxy(jwtService, chatService)
	r.Route("/centrifugo/proxy", func(r chi.Router) {
		r.Post("/connect", centrifugoProxy.HandleConnect)
		r.Post("/subscribe", centrifugoProxy.HandleSubscribe)
	})

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.JWTAuth(jwtService))
		r.Route("/api/v1", func(r chi.Router) {
			chatHandler.RegisterRoutes(r)
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
	}

	// Kafka consumer
	messageConsumer, err := chatkafka.NewConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.ConsumerGroup,
		[]string{events.TopicMessagesInbound},
		func(ctx context.Context, record *kgo.Record) error {
			envelope, err := events.DecodeEnvelope(record.Value)
			if err != nil {
				return err
			}
			log.Debug().
				Str("event_type", envelope.EventType).
				Str("event_id", envelope.EventID).
				Msg("processing kafka message")
			return nil
		},
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create kafka consumer")
	}

	return &Application{
		HTTPServer:      srv,
		MessageConsumer: messageConsumer,
		PresenceStore:   presenceStore,
	}
}

func (a *Application) Start(ctx context.Context) {
	// Start Kafka consumer
	go func() {
		if err := a.MessageConsumer.Start(ctx); err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("kafka consumer error")
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
					log.Error().Err(err).Msg("presence cleanup error")
				}
			}
		}
	}()

	// Start HTTP server
	go func() {
		log.Info().Str("addr", a.HTTPServer.Addr).Msg("http server started")
		if err := a.HTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http server error")
		}
	}()
}

func (a *Application) Shutdown(ctx context.Context) {
	if err := a.HTTPServer.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("http server shutdown error")
	}
	a.MessageConsumer.Close()
}
