package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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

// logFilePath resolves where the relay writes its own log, if anywhere.
// LOG_PATH wins so a deployment can set it without editing config.
func logFilePath(cfg *config.Config) string {
	if p := strings.TrimSpace(os.Getenv("LOG_PATH")); p != "" {
		return p
	}
	if cfg != nil {
		return strings.TrimSpace(cfg.Server.LogPath)
	}
	return ""
}

// openLogFile opens the log for appending, creating the directory if needed.
//
// Appends rather than truncating: restarting the relay must not erase the
// record of why it was restarted.
func openLogFile(path string) (*os.File, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
	}
	// 0640: readable by the service account and its group, not world-readable.
	// These logs carry agent ids, hostnames and client IPs.
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
}

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
		//
		// Also tee to a FILE when one is configured. Container stdout is the
		// only sink otherwise, so logs live and die with the container: a
		// `docker compose up --force-recreate` discards the record of what
		// happened before it, which is exactly the window an incident review
		// needs. A path on a mounted volume survives restarts, redeploys and
		// image rebuilds.
		out := io.Writer(os.Stdout)
		if path := logFilePath(cfg); path != "" {
			if w, err := openLogFile(path); err != nil {
				// Never fail startup for logging: a relay that refuses to boot
				// because a log directory is missing is worse than one that
				// logs only to stdout.
				log.Warn().Err(err).Str("path", path).
					Msg("could not open the log file; logging to stdout only")
			} else {
				out = io.MultiWriter(os.Stdout, w)
			}
		}
		log.Logger = zerolog.New(out).With().Timestamp().Logger()
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
