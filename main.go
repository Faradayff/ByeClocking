package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Faradayff/ByeClocking/clockers"
)

// main is the entry point of the ByeClocking application.
func main() {
	logLevel := flag.String("loglevel", "DEBUG", "Log level: DEBUG, INFO, WARN or ERROR")
	flag.Parse()

	initLogging(*logLevel)

	slog.Info("Starting application")

	cfg, err := LoadConfig("config.json")
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var clocker Clocker
	switch cfg.ClockingPlatform {
	case "myteam2go":
		clocker = clockers.NewMyTeam2GoClocker(cfg.CompanyName, cfg.Account, cfg.Password)
	default:
		clocker = &DummyClocker{}
	}

	Run(ctx, cfg, clocker)

	slog.Info("Application shut down gracefully")
}
