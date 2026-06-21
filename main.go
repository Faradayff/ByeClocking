package main

import (
	"flag"
	"log"
	"log/slog"
	"math/rand"
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

	for {
		// TODO: implement summer time
		slog.Info("Starting the day")
		clockInTime, lunchTime, lunchFinishTime, clockOutTime := randomizeHours(cfg)

		signalToClock := make(chan bool)

		go waitingTicker(clockInTime, signalToClock)
		slog.Debug("Waiting to clock in")

		<-signalToClock
		slog.Info("Clock in time")

		// clockIn

		go waitingTicker(lunchTime, signalToClock)
		slog.Debug("Waiting to go to lunch")

		<-signalToClock
		slog.Info("Lunch time")

		// clockPause

		go waitingTicker(lunchFinishTime, signalToClock)
		slog.Debug("Waiting to go back from lunch")

		<-signalToClock
		slog.Info("Back from lunch time")

		// clockResume

		go waitingTicker(clockOutTime, signalToClock)
		slog.Debug("Waiting to clock out")

		<-signalToClock
		slog.Info("Clock out time")

		// clockOut

		waitUntilTomorrow(cfg.ClockIn.Time)
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

func waitingTicker(targetHour time.Time, signal chan<- bool) {
	timeToClock := time.Until(targetHour)
	slog.Debug("Waiting Ticker is going to wait", "duration", timeToClock.Minutes())
	<-time.After(timeToClock)
	signal <- true
}

func waitUntilTomorrow(clockIn time.Time) {
	wakeUpTime := time.Until(clockIn) - time.Hour
	slog.Debug("Waiting until tomorrow", "duration", wakeUpTime.Minutes())
	<-time.After(wakeUpTime)
}

func randomizeHours(cfg *Config) (time.Time, time.Time, time.Time, time.Time) {
	slog.Info("Setting up delays and times")
	clockInDelay, lunchDelay, lunchDuration, clockOutDelay := initDelays(cfg)

	clockInTime := cfg.ClockIn.Time.Add(clockInDelay * time.Minute)
	lunchTime := cfg.Lunchtime.Time.Add(lunchDelay * time.Minute)
	lunchFinishTime := cfg.Lunchtime.Time.Add(lunchDuration * time.Minute)
	clockOutTime := cfg.ClockOut.Time.Add(clockOutDelay * time.Minute)

	slog.Debug("Times initialized", "clockInTime", clockInTime, "lunchTime", lunchTime, "lunchFinishTime", lunchFinishTime, "clockOutTime", clockOutTime)
	return clockInTime, lunchTime, lunchFinishTime, clockOutTime
}

func initDelays(cfg *Config) (clockInDelay time.Duration, lunchDelay time.Duration, lunchDuration time.Duration, clockOutDelay time.Duration) {
	if cfg.Unpunctuality > 0 {
		clockInDelay = time.Duration(rand.Intn(cfg.Unpunctuality))
		clockOutDelay = time.Duration(rand.Intn(cfg.LeaveUnpunctuality)) + clockInDelay
	}
	if cfg.LunchUnpunctuality > 0 {
		lunchDelay = time.Duration(rand.Intn(cfg.LunchUnpunctuality))
		lunchDuration = time.Duration(cfg.MinTimeToLunch + rand.Intn(cfg.MaxTimeToLunch-cfg.MinTimeToLunch+1))
	}

	slog.Debug("Delays initialized", "ClockInDelay", clockInDelay, "lunchDelay", lunchDelay, "lunchDuration", lunchDuration, "clockOutDelay", clockOutDelay)
	return
}
