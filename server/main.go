package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/server/api"
	serverauth "github.com/neev/remote-agent/server/auth"
	"github.com/neev/remote-agent/server/config"
	"github.com/neev/remote-agent/server/session"
	"github.com/neev/remote-agent/server/signaling"
)

func main() {
	// Logging setup will be configured after loading config

	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	if cfg.Server.Debug {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		// Production audit logging — structured JSON
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Redis connection.
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Str("addr", cfg.Redis.Addr).Msg("cannot connect to Redis")
	}
	log.Info().Str("addr", cfg.Redis.Addr).Msg("connected to Redis")

	// Build layers.
	authStore := serverauth.NewStore(rdb)
	if cfg.Auth.Enabled {
		if err := authStore.EnsureBootstrapUser(ctx, cfg.Auth); err != nil {
			log.Fatal().Err(err).Msg("failed to prepare dashboard authentication")
		}
	}
	clientCA, err := serverauth.LoadOrCreateClientCA(cfg.Server.TLSClientCA, cfg.Server.TLSClientCAKey)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize client CA")
	}
	registry := session.NewRegistry(rdb)
	hub := signaling.NewHub(registry, cfg, clientCA)

	// Start WebSocket ping loop.
	go hub.RunPinger(30 * time.Second)

	// HTTP server.
	srv := api.New(cfg, registry, hub, authStore)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Info().Str("addr", addr).Msg("starting signaling server")

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenDual(addr); err != nil {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-quit
	log.Info().Msg("shutting down…")
	os.Exit(0)
}
