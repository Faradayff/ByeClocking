package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClockTimeJSON(t *testing.T) {
	t.Run("Unmarshal Valid", func(t *testing.T) {
		jsonData := []byte(`"14:30"`)
		var ct ClockTime
		err := json.Unmarshal(jsonData, &ct)
		require.NoError(t, err)
		assert.Equal(t, 14, ct.Time.Hour())
		assert.Equal(t, 30, ct.Time.Minute())
	})

	t.Run("Unmarshal Invalid", func(t *testing.T) {
		jsonData := []byte(`"invalid"`)
		var ct ClockTime
		err := json.Unmarshal(jsonData, &ct)
		assert.Error(t, err)
	})

	t.Run("Unmarshal Empty", func(t *testing.T) {
		jsonData := []byte(`""`)
		var ct ClockTime
		err := json.Unmarshal(jsonData, &ct)
		require.NoError(t, err)
		assert.True(t, ct.Time.IsZero())
	})

	t.Run("Marshal", func(t *testing.T) {
		ct := ClockTime{Time: time.Date(0, 1, 1, 9, 5, 0, 0, time.UTC)}
		b, err := json.Marshal(&ct)
		require.NoError(t, err)
		assert.Equal(t, `"09:05"`, string(b))
	})
}

func TestValidateRequiredFields(t *testing.T) {
	t.Run("Missing Platform", func(t *testing.T) {
		cfg := Config{}
		err := cfg.validateRequiredFields()
		assert.EqualError(t, err, "clocking platform is empty")
	})

	t.Run("MyTeam2Go Missing Credentials", func(t *testing.T) {
		cfg := Config{ClockingPlatform: "myteam2go"}
		err := cfg.validateRequiredFields()
		assert.EqualError(t, err, "factorial.api_key is empty")

		cfg.MyTeam2Go.Account = "acc"
		cfg.MyTeam2Go.Password = "pass"
		err = cfg.validateRequiredFields()
		assert.EqualError(t, err, "company name is empty")
	})

	t.Run("Valid", func(t *testing.T) {
		cfg := Config{ClockingPlatform: "other"}
		err := cfg.validateRequiredFields()
		assert.NoError(t, err)
	})
}

func TestNormalizations(t *testing.T) {
	cfg := Config{
		Unpunctuality:      -5,
		LeaveUnpunctuality: -10,
		LunchUnpunctuality: -2,
	}

	cfg.normalizeUnpunctuality()
	cfg.normalizeLeaveUnpunctuality()
	cfg.normalizeLunchUnpunctuality()

	assert.Equal(t, 0, cfg.Unpunctuality)
	assert.Equal(t, 0, cfg.LeaveUnpunctuality)
	assert.Equal(t, 0, cfg.LunchUnpunctuality)
}

func TestValidateLunchSettings(t *testing.T) {
	lunchtime := &ClockTime{Time: time.Date(0, 1, 1, 14, 0, 0, 0, time.UTC)}

	t.Run("MaxTimeToLunch <= 0", func(t *testing.T) {
		cfg := Config{Lunchtime: lunchtime, MaxTimeToLunch: 0}
		cfg.validateLunchSettings()
		assert.Nil(t, cfg.Lunchtime)
	})

	t.Run("MinTimeToLunch < 1", func(t *testing.T) {
		cfg := Config{Lunchtime: lunchtime, MaxTimeToLunch: 60, MinTimeToLunch: 0}
		cfg.validateLunchSettings()
		assert.Nil(t, cfg.Lunchtime)
	})

	t.Run("Min > Max Swap", func(t *testing.T) {
		cfg := Config{Lunchtime: lunchtime, MinTimeToLunch: 60, MaxTimeToLunch: 30}
		cfg.validateLunchSettings()
		assert.Equal(t, 30, cfg.MinTimeToLunch)
		assert.Equal(t, 60, cfg.MaxTimeToLunch)
	})
}

func TestValidateSummerSettings(t *testing.T) {
	t.Run("Invalid Period", func(t *testing.T) {
		cfg := Config{SummerPeriod: []string{"01/06"}}
		cfg.validateSummerSettings()
		assert.Nil(t, cfg.SummerPeriod)
	})

	t.Run("Missing Summer Times", func(t *testing.T) {
		cfg := Config{SummerPeriod: []string{"01/06", "31/08"}, SummerTimes: []ClockTime{}}
		cfg.validateSummerSettings()
		assert.Nil(t, cfg.SummerPeriod)
	})
}

func TestValidateFridaySettings(t *testing.T) {
	t.Run("Invalid Length", func(t *testing.T) {
		cfg := Config{FridayTimes: []ClockTime{{}}}
		cfg.validateFridaySettings()
		assert.Nil(t, cfg.FridayTimes)
	})

	t.Run("Valid Length", func(t *testing.T) {
		cfg := Config{FridayTimes: []ClockTime{{}, {}}}
		cfg.validateFridaySettings()
		assert.Len(t, cfg.FridayTimes, 2)
	})
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "config.json")

	validJSON := `{
		"clock_in": "09:00",
		"clock_out": "18:00",
		"clocking_platform": "test_platform"
	}`

	err := os.WriteFile(filePath, []byte(validJSON), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(filePath)
	require.NoError(t, err)
	assert.Equal(t, "test_platform", cfg.ClockingPlatform)
}

func TestLoadConfigErrors(t *testing.T) {
	t.Run("File Not Found", func(t *testing.T) {
		_, err := LoadConfig("non_existent.json")
		assert.Error(t, err)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "config.json")
		err := os.WriteFile(filePath, []byte("{invalid"), 0644)
		require.NoError(t, err)

		_, err = LoadConfig(filePath)
		assert.Error(t, err)
	})
}
