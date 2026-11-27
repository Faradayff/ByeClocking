package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Faradayff/ByeClocking/internal/clients"
	"github.com/Faradayff/ByeClocking/internal/config"
	"github.com/Faradayff/ByeClocking/internal/core"
	"github.com/Faradayff/ByeClocking/internal/logger"
	"github.com/Faradayff/ByeClocking/internal/webhook"
)

// main is the entry point of the ByeClocking application.
func main() {
	// The -loglevel flag keeps working as an override; its default now comes
	// from the $LOG_LEVEL environment variable (see the .env file).
	logLevel := flag.String("loglevel", defaultLogLevel(), "Log level: DEBUG, INFO, WARN or ERROR (default from $LOG_LEVEL, else INFO)")
	flag.Parse()

	closeLogger := logger.InitLogging(*logLevel)
	defer closeLogger()

	// Build the webhook notifier from the WEBHOOK_* environment variables. It is
	// a no-op when WEBHOOK_URL is not set, so this is safe to always do. It only
	// errors out when a variable is present but invalid.
	notifier, err := webhook.FromEnv()
	if err != nil {
		// The notifier itself is broken here, so we can (and should) only log.
		slog.Error("❌ Failed to configure the webhook notifier", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	// One line, both channels (log + webhook): startup.
	core.Announce(ctx, notifier, slog.LevelInfo, "🚀 Starting application")

	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		core.Announce(ctx, notifier, slog.LevelError, "❌ Failed to load config: "+err.Error())
		os.Exit(1)
	}

	clocker := buildClocker(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	core.Run(ctx, cfg, clocker, notifier)

	core.Announce(ctx, notifier, slog.LevelInfo, "👋 Application shut down gracefully")
}

// defaultLogLevel returns the log level set in $LOG_LEVEL (case-insensitive),
// or the previous default of "INFO" when it is not present.
func defaultLogLevel() string {
	if v := strings.ToUpper(strings.TrimSpace(os.Getenv("LOG_LEVEL"))); v != "" {
		return v
	}
	return "INFO"
}

// buildClocker creates the appropriate Clocker implementation based on the configured platform.
func buildClocker(cfg *config.Config) clients.Clocker {
	switch strings.ToLower(cfg.ClockingPlatform) {
	case "myteam2go":
		slog.Debug("🔍 Initialising MyTeam2Go clocker")
		return clients.NewMyTeam2GoClocker(cfg.ClientConfig["company_name"], cfg.ClientConfig["account"], cfg.ClientConfig["password"], cfg.Latitude, cfg.Longitude)
	default:
		slog.Warn("⚠️ Unknown clocking platform, using DummyClocker", "platform", cfg.ClockingPlatform)
		return &clients.DummyClocker{}
	}
}
