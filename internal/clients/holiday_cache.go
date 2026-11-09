package clients

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"
)

const holidayCachePath = "cache/holidays_cache.json"

// holidayRange represents a single approved vacation period.
type holidayRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// holidayCache is the structure persisted to disk.
type holidayCache struct {
	Holidays     []holidayRange `json:"holidays"`
	BankHolidays []time.Time    `json:"bank_holidays"`
}

// loadHolidayCache reads the cache file from disk and prunes expired vacation entries.
// Returns an empty cache if the file does not exist or cannot be parsed.
func loadHolidayCache() holidayCache {
	data, err := os.ReadFile(holidayCachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("⚠️ IsHoliday: could not read holiday cache", "error", err)
		}
		return holidayCache{}
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return holidayCache{}
	}

	var cache holidayCache
	if err := json.Unmarshal(data, &cache); err != nil {
		slog.Warn("⚠️ IsHoliday: holiday cache is malformed, ignoring", "error", err)
		return holidayCache{}
	}

	cache = pruneExpiredRanges(cache)
	return cache
}

// saveHolidayCache prunes expired ranges, writes the cache to disk, and returns
// the pruned cache for immediate use by the caller (avoiding a second disk read).
func saveHolidayCache(cache holidayCache) holidayCache {
	cache = pruneExpiredRanges(cache)

	if err := os.MkdirAll("cache", 0o755); err != nil {
		slog.Warn("⚠️ IsHoliday: could not create cache directory", "error", err)
		return cache
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		slog.Warn("⚠️ IsHoliday: could not marshal holiday cache", "error", err)
		return cache
	}

	if err := os.WriteFile(holidayCachePath, data, 0o644); err != nil {
		slog.Warn("⚠️ IsHoliday: could not write holiday cache", "error", err)
		return cache
	}
	slog.Debug("💾 Holiday cache saved", "path", holidayCachePath, "holidays", len(cache.Holidays), "bank_holidays", len(cache.BankHolidays))
	return cache
}

// pruneExpiredRanges removes vacation ranges whose end date is strictly before today (midnight).
func pruneExpiredRanges(cache holidayCache) holidayCache {
	today := time.Now()
	todayMidnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	var active []holidayRange
	for _, r := range cache.Holidays {
		if !r.End.Before(todayMidnight) {
			active = append(active, r)
		}
	}

	if pruned := len(cache.Holidays) - len(active); pruned > 0 {
		slog.Debug("🗑️ Pruned expired vacation ranges from cache", "count", pruned)
	}

	cache.Holidays = active
	return cache
}

// hasBankHolidaysForYear checks if cache has any bank holidays for the given year.
func hasBankHolidaysForYear(cache holidayCache, year int) bool {
	for _, bh := range cache.BankHolidays {
		if bh.Year() == year {
			return true
		}
	}
	return false
}

// isBankHolidayInCache checks if the given date matches any cached bank holiday.
func isBankHolidayInCache(cache holidayCache, date time.Time) bool {
	y, m, d := date.Date()
	for _, bh := range cache.BankHolidays {
		by, bm, bd := bh.Date()
		if y == by && m == bm && d == bd {
			return true
		}
	}
	return false
}

// isHolidayInCache returns true if today falls within any cached vacation range.
func isHolidayInCache(cache holidayCache) bool {
	today := time.Now()
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	for _, r := range cache.Holidays {
		if !todayDate.Before(r.Start) && !todayDate.After(r.End) {
			return true
		}
	}
	return false
}

// fallbackToCache loads the holiday cache from disk and checks if today is a holiday
// (either bank holiday or approved personal vacation).
// Returns false if the cache is empty or unavailable.
func fallbackToCache(reason string) bool {
	cache := loadHolidayCache()
	today := time.Now()

	if isBankHolidayInCache(cache, today) {
		slog.Warn("⚠️ IsHoliday: using cached bank holiday data", "reason", reason)
		return true
	}

	if len(cache.Holidays) == 0 {
		slog.Debug("📭 Holiday cache is empty, assuming not a holiday", "reason", reason)
		return false
	}
	slog.Warn("⚠️ IsHoliday: using cached holiday data", "reason", reason)
	return isHolidayInCache(cache)
}
