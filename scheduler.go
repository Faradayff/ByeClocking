package main

import (
	"log/slog"
	"math/rand"
)

func initDelays(cfg *Config) (int, int) {
	slog.Info("Setting up delays and times")

	var delay, lunchDelay int
	if cfg.Unpunctuality > 0 {
		delay = rand.Intn(cfg.Unpunctuality)
	} else {
		delay = -1
	}
	if cfg.LunchUnpunctuality > 0 {
		lunchDelay = rand.Intn(cfg.LunchUnpunctuality)
	} else {
		lunchDelay = -1
	}

	slog.Debug("Delays initialized", "delay", delay, "lunchDelay", lunchDelay)
	return delay, lunchDelay
}
