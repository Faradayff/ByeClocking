package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

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

	Run(ctx, cfg, &DummyClocker{})

	slog.Info("Application shut down gracefully")
}
