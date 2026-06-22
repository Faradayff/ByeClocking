package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type ClockTime struct {
	time.Time
}

// UnmarshalJSON parses a time string in "HH:mm" format into a ClockTime object.
func (ct *ClockTime) UnmarshalJSON(data []byte) error {
	value := strings.Trim(string(data), `"`)

	if value == "" || value == "null" {
		return nil
	}

	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return fmt.Errorf("invalid time %q, expected HH:mm: %w", value, err)
	}

	ct.Time = parsed
	return nil
}

// MarshalJSON converts a ClockTime object back into a JSON string format ("HH:mm").
func (ct *ClockTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(ct.Format("15:04"))
}

type Config struct {
	ClockingPlatform   string      `json:"clocking_platform"`
	Account            string      `json:"account"`
	Password           string      `json:"password"`
	CompanyName        string      `json:"company_name"`
	ClockIn            ClockTime   `json:"clock_in"`
	ClockOut           ClockTime   `json:"clock_out"`
	Unpunctuality      int         `json:"unpunctuality"`
	LeaveUnpunctuality int         `json:"leave_unpunctuality"`
	Lunchtime          *ClockTime  `json:"lunchtime"`
	MinTimeToLunch     int         `json:"min_time_to_lunch"`
	MaxTimeToLunch     int         `json:"max_time_to_lunch"`
	LunchUnpunctuality int         `json:"lunch_unpunctuality"`
	SummerTimes        []ClockTime `json:"summer_times"`
	SummerPeriod       []string    `json:"summer_period"`
}

// LoadConfig reads, parses, and validates the configuration from the specified JSON file.
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

// validate ensures that the configuration values are correct, consistent, and normalized.
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

// validateRequiredFields checks if all mandatory configuration fields are present.
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

// requiredStringError logs and returns an error for a missing required string field.
func requiredStringError(displayName, errorName string) error {
	slog.Error(displayName + " is empty. Stopping")
	return errors.New(errorName + " is empty")
}

// normalizeUnpunctuality ensures the unpunctuality value is not negative, resetting it to zero if it is.
func (cfg *Config) normalizeUnpunctuality() {
	if cfg.Unpunctuality >= 0 {
		return
	}

	slog.Warn("Unpunctuality is negative. Setting it to zero")
	cfg.Unpunctuality = 0
}

// normalizeLeaveUnpunctuality ensures the leave unpunctuality value is not negative, resetting it to zero if it is.
func (cfg *Config) normalizeLeaveUnpunctuality() {
	if cfg.LeaveUnpunctuality >= 0 {
		return
	}

	slog.Warn("Leave Unpunctuality is negative. Setting it to zero")
	cfg.LeaveUnpunctuality = 0
}

// validateLunchSettings checks the consistency of lunch-related configuration parameters.
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

// normalizeLunchUnpunctuality ensures the lunch unpunctuality value is not negative, resetting it to zero if it is.
func (cfg *Config) normalizeLunchUnpunctuality() {
	if cfg.LunchUnpunctuality >= 0 {
		return
	}

	slog.Warn("Lunch unpunctuality is negative. Setting it to zero")
	cfg.LunchUnpunctuality = 0
}

// validateSummerSettings verifies the summer period and times configuration for validity.
func (cfg *Config) validateSummerSettings() {
	if len(cfg.SummerPeriod) != 2 || cfg.SummerPeriod[0] == "" || cfg.SummerPeriod[1] == "" {
		slog.Warn("Summer period hasn't two dates. Disabling it")
		cfg.SummerPeriod = nil
		return
	}

	if len(cfg.SummerPeriod) > 0 && len(cfg.SummerTimes) == 0 {
		slog.Warn("Summer period is set but summer times are empty. Disabling summer period", "summerPeriod", cfg.SummerPeriod)
		cfg.SummerPeriod = nil
	}
}
