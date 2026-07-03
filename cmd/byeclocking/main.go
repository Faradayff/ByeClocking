package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Faradayff/ByeClocking/internal/clients"
	"github.com/Faradayff/ByeClocking/internal/config"
	"github.com/Faradayff/ByeClocking/internal/core"
	"github.com/Faradayff/ByeClocking/internal/logger"
)

// main is the entry point of the ByeClocking application.
func main() {
	logLevel := flag.String("loglevel", "DEBUG", "Log level: DEBUG, INFO, WARN or ERROR")
	flag.Parse()

	logger.InitLogging(*logLevel)

	slog.Info("Starting application")

	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	core.Run(ctx, cfg, &clients.DummyClocker{})

	slog.Info("Application shut down gracefully")
}
