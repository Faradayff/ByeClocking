package main

import (
	"flag"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	logDir = "."
)

func main() {
	logLevel := flag.String("loglevel", "DEBUG", "Log level: DEBUG, INFO, WARN or ERROR")
	flag.Parse()

	initLogging(*logLevel)

	slog.Debug("Starting application")

	cfg, err := LoadConfig("config.json")
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	initDelays(cfg)

	slog.Debug("Configuration finished")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// TODO: randomize hours

	for tick := range ticker.C {
		// TODO: implement summer time
		if tick.Hour() == cfg.ClockIn.Hour() {
			slog.Debug("Clock in time")
		} else if tick.Hour() == cfg.ClockOut.Hour() {
			slog.Debug("Clock out time")
		} else if tick.Hour() == cfg.Lunchtime.Hour() {
			slog.Debug("Lunch time")
		} else {
			slog.Debug("Back from lunch time")
		}
		os.Exit(0)
	}
}

func initLogging(loglevel string) {
	opts := &slog.HandlerOptions{}

	switch loglevel {
	case "DEBUG":
		opts.Level = slog.LevelDebug
	case "INFO":
		opts.Level = slog.LevelInfo
	case "WARN":
		opts.Level = slog.LevelWarn
	case "ERROR":
		opts.Level = slog.LevelError
	default:
		opts.Level = slog.LevelInfo
	}

	logFilePath := filepath.Join(logDir, "ByeClocking.log")

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	handler := slog.NewTextHandler(logFile, opts)

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
