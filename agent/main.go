package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/bootstrap"
	"github.com/neev/remote-agent/agent/capture"
	"github.com/neev/remote-agent/agent/core"
	"github.com/neev/remote-agent/agent/session"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Phase 0 transport split (opt-in via flags; default = existing all-in-one
	// host, so current behavior is unchanged):
	//   --transport       persistent process that owns the WebRTC connection
	//   --capture-worker  per-session process that captures + streams frames
	if mode := parseMode(); mode != "" {
		// --relay <url> overrides RELAY_URL (the SYSTEM service passes it so the
		// session-0 transport reaches the same relay as the Flutter installer).
		if relay := parseFlagValue("--relay"); relay != "" {
			_ = os.Setenv("RELAY_URL", relay)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			<-quit
			cancel()
		}()
		var err error
		switch mode {
		case "transport":
			err = session.RunTransport(ctx, 0)
		case "capture-worker":
			err = session.RunCaptureWorker(ctx, 0)
		}
		if err != nil && err != context.Canceled {
			log.Fatal().Err(err).Str("mode", mode).Msg("session process exited")
		}
		return
	}

	bootstrapCfg, err := bootstrap.Load()
	if err != nil {
		log.Warn().Err(err).Msg("failed to load bootstrap config; falling back to environment")
	}
	applyBootstrapEnv(bootstrapCfg)

	if runtime.GOOS == "darwin" {
		if err := capture.RequestScreenCapturePermission(); err != nil {
			log.Warn().Err(err).Msg("macOS screen recording permission not granted yet")
		}
	}

	relayURL := bootstrapCfg.RelayURL

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	unattendedPassword := bootstrapCfg.UnattendedPassword
	instance, err := core.StartAgent(ctx, relayURL, unattendedPassword, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start agent")
	}
	defer instance.Stop()

	// Start local API server so browser UI can fetch agent info.
	platform := runtime.GOOS + "/" + runtime.GOARCH
	api, err := core.StartLocalAPI(instance, platform)
	if err != nil {
		log.Warn().Err(err).Msg("Local API server failed to start")
	} else {
		defer api.Stop()
	}

	// Wait for the agent to register with the signaling server and get its ID.
	// Registration is async so this may take a moment.
	instance.WaitForRegistered()

	fmt.Printf("\n")
	fmt.Printf("╔══════════════════════════════════════╗\n")
	fmt.Printf("║        REMOTE AGENT READY 🚀          ║\n")
	fmt.Printf("╠══════════════════════════════════════╣\n")
	fmt.Printf("║  Agent ID  :  %-22s ║\n", instance.GetID())
	fmt.Printf("║  Password  :  %-22s ║\n", instance.Password)
	fmt.Printf("║  Platform  :  %-22s ║\n", platform)
	if api != nil {
		fmt.Printf("║  API Port  :  %-22d ║\n", api.Port())
	}
	fmt.Printf("╚══════════════════════════════════════╝\n")
	fmt.Printf("\n")

	// Headless by default. Only open a UI when explicitly requested.
	uiURL := strings.TrimSpace(os.Getenv("UI_URL"))
	if uiURL != "" && os.Getenv("NO_BROWSER") == "" {
		go openBrowser(uiURL)
	} else {
		log.Info().Str("ui_url", uiURL).Msg("browser auto-open disabled")
	}

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	log.Info().Msg("shutting down…")
	cancel()
	time.Sleep(500 * time.Millisecond)
}

// parseMode returns "transport", "capture-worker", or "" (default all-in-one).
func parseMode() string {
	for _, a := range os.Args[1:] {
		switch a {
		case "--transport":
			return "transport"
		case "--capture-worker":
			return "capture-worker"
		}
	}
	return ""
}

// parseFlagValue returns the argument following [flag] (e.g. --relay <url>).
func parseFlagValue(flag string) string {
	args := os.Args[1:]
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func applyBootstrapEnv(cfg bootstrap.Config) {
	if cfg.EnrollmentCode != "" {
		_ = os.Setenv("ENROLLMENT_CODE", cfg.EnrollmentCode)
	}
	if cfg.OrgID != "" {
		_ = os.Setenv("ORG_ID", cfg.OrgID)
	}
	if cfg.DeviceGroup != "" {
		_ = os.Setenv("DEVICE_GROUP", cfg.DeviceGroup)
	}
	if cfg.TURNURL != "" {
		_ = os.Setenv("TURN_URL", cfg.TURNURL)
	}
	if cfg.TURNUser != "" {
		_ = os.Setenv("TURN_USER", cfg.TURNUser)
	}
	if cfg.TURNPass != "" {
		_ = os.Setenv("TURN_PASS", cfg.TURNPass)
	}
	if cfg.CertFile != "" {
		_ = os.Setenv("AGENT_CERT_FILE", cfg.CertFile)
	}
	if cfg.KeyFile != "" {
		_ = os.Setenv("AGENT_KEY_FILE", cfg.KeyFile)
	}
	if cfg.CAFile != "" {
		_ = os.Setenv("AGENT_CA_FILE", cfg.CAFile)
	}
	if cfg.UnattendedPassword != "" {
		_ = os.Setenv("UNATTENDED_PASSWORD", cfg.UnattendedPassword)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	if err := cmd.Start(); err != nil {
		log.Warn().Err(err).Msg("failed to open browser")
	}
}
