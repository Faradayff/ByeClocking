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
)

// main is the entry point of the ByeClocking application.
func main() {
	logLevel := flag.String("loglevel", "INFO", "Log level: DEBUG, INFO, WARN or ERROR")
	flag.Parse()

	closeLogger := logger.InitLogging(*logLevel)
	defer closeLogger()

	slog.Info("🚀 Starting application")

	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		slog.Error("❌ Failed to load config", "error", err)
		os.Exit(1)
	}

	clocker := buildClocker(cfg)
	slog.Info("🔧 Using clocking platform", "platform", cfg.ClockingPlatform)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	core.Run(ctx, cfg, clocker)

	slog.Info("👋 Application shut down gracefully")
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
