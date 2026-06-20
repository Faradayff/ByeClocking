package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

type Config struct {
	ClockingPlatform   string      `json:"Clocking_platform"`
	Account            string      `json:"account"`
	Password           string      `json:"password"`
	CompanyName        string      `json:"company_name"`
	ClockIn            time.Time   `json:"clock_in"`
	ClockOut           time.Time   `json:"clock_out"`
	Unpunctuality      int         `json:"unpunctuality"`
	LeaveUnpunctuality int         `json:"leave_unpunctuality"`
	Lunchtime          *time.Time  `json:"lunchtime"`
	MinTimeToLunch     int         `json:"min_time_to_lunch"`
	MaxTimeToLunch     int         `json:"max_time_to_lunch"`
	LunchUnpunctuality int         `json:"lunch_unpunctuality"`
	SummerTimes        []time.Time `json:"summer_times"`
	SummerPeriod       []time.Time `json:"summer_period"`
}

func LoadConfig(filePath string) (*Config, error) {
	slog.Debug("Getting config from file")

	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error opening config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(file, &cfg); err != nil {
		return nil, fmt.Errorf("error decoding json: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	slog.Debug("Config loaded", "cfg", cfg)

	return &cfg, nil
}

func (cfg *Config) validate() error {
	if err := cfg.validateRequiredFields(); err != nil {
		return err
	}

	cfg.normalizeUnpunctuality()
	cfg.normalizeLeaveUnpunctuality()
	cfg.validateLunchSettings()
	cfg.normalizeLunchUnpunctuality()
	cfg.validateSummerSettings()

	return nil
}

func (cfg *Config) validateRequiredFields() error {
	if cfg.ClockingPlatform == "" {
		return requiredStringError("Clocking platform", "clocking platform")
	}
	if cfg.Account == "" {
		return requiredStringError("Account", "account")
	}
	if cfg.Password == "" {
		return requiredStringError("Password", "password")
	}
	if cfg.CompanyName == "" {
		return requiredStringError("Company name", "company name")
	}

	return nil
}

func requiredStringError(displayName, errorName string) error {
	slog.Error(displayName + " is empty. Stopping")
	return errors.New(errorName + " is empty")
}

func (cfg *Config) normalizeUnpunctuality() {
	if cfg.Unpunctuality >= 0 {
		return
	}

	slog.Warn("Unpunctuality is negative. Setting it to zero")
	cfg.Unpunctuality = 0
}

func (cfg *Config) normalizeLeaveUnpunctuality() {
	if cfg.LeaveUnpunctuality >= 0 {
		return
	}

	slog.Warn("Leave Unpunctuality is negative. Setting it to zero")
	cfg.LeaveUnpunctuality = 0
}

func (cfg *Config) validateLunchSettings() {
	const minimumLunchDuration = 1

	if cfg.Lunchtime == nil {
		return
	}

	if cfg.MaxTimeToLunch <= 0 {
		slog.Warn("Max time to lunch is 0 or negative. Disabling Lunch Time", "maxTimeToLunch", cfg.MaxTimeToLunch)
		cfg.Lunchtime = nil
		return
	}

	if cfg.MinTimeToLunch < minimumLunchDuration {
		slog.Error("Min time to lunch can't be less than 1. Disabling Lunch Time", "minTimeToLunch", cfg.MinTimeToLunch)
		cfg.Lunchtime = nil
		return
	}

	if cfg.MinTimeToLunch > cfg.MaxTimeToLunch {
		slog.Warn(
			"Min time to lunch is greater than max time to lunch, switching values",
			"minTimeToLunch", cfg.MinTimeToLunch,
			"maxTimeToLunch", cfg.MaxTimeToLunch,
		)
		cfg.MinTimeToLunch, cfg.MaxTimeToLunch = cfg.MaxTimeToLunch, cfg.MinTimeToLunch
	}
}

func (cfg *Config) normalizeLunchUnpunctuality() {
	if cfg.LunchUnpunctuality >= 0 {
		return
	}

	slog.Warn("Lunch unpunctuality is negative. Setting it to zero")
	cfg.LunchUnpunctuality = 0
}

func (cfg *Config) validateSummerSettings() {
	if len(cfg.SummerPeriod)%2 != 0 {
		slog.Warn("Summer period is not even. Disabling it")
		cfg.SummerPeriod = nil
		return
	}

	if len(cfg.SummerPeriod) > 0 && len(cfg.SummerTimes) == 0 {
		slog.Warn("Summer period is set but summer times are empty. Disabling summer period", "summerPeriod", cfg.SummerPeriod)
		cfg.SummerPeriod = nil
	}
}
