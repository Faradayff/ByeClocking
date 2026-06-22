package main

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	logDir = "."
)

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
	defer logFile.Close()

	var handler slog.Handler
	if loglevel == "DEBUG" {
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		handler = slog.NewTextHandler(multiWriter, opts)
	} else {
		handler = slog.NewTextHandler(logFile, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
