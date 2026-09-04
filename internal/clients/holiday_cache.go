package clients

import (
	"encoding/json"
	"log/slog"
	"os"
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
	Ranges []holidayRange `json:"ranges"`
}

// loadHolidayCache reads the cache file from disk and prunes expired entries.
// Returns an empty cache if the file does not exist or cannot be parsed.
func loadHolidayCache() holidayCache {
	data, err := os.ReadFile(holidayCachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("⚠️ IsHoliday: could not read holiday cache", "error", err)
		}
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
	slog.Debug("💾 Holiday cache saved", "path", holidayCachePath, "ranges", len(cache.Ranges))
	return cache
}

// pruneExpiredRanges removes vacation ranges whose end date is strictly before today (midnight).
func pruneExpiredRanges(cache holidayCache) holidayCache {
	today := time.Now()
	todayMidnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	var active []holidayRange
	for _, r := range cache.Ranges {
		if !r.End.Before(todayMidnight) {
			active = append(active, r)
		}
	}

	if pruned := len(cache.Ranges) - len(active); pruned > 0 {
		slog.Debug("🗑️ Pruned expired vacation ranges from cache", "count", pruned)
	}

	cache.Ranges = active
	return cache
}

// isHolidayInCache returns true if today falls within any cached vacation range.
func isHolidayInCache(cache holidayCache) bool {
	today := time.Now()
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	for _, r := range cache.Ranges {
		if !todayDate.Before(r.Start) && !todayDate.After(r.End) {
			return true
		}
	}
	return false
}

// fallbackToCache loads the holiday cache from disk and checks if today is a holiday.
// Returns false if the cache is empty or unavailable.
func fallbackToCache(reason string) bool {
	cache := loadHolidayCache()
	if len(cache.Ranges) == 0 {
		slog.Debug("📭 Holiday cache is empty, assuming not a holiday", "reason", reason)
		return false
	}
	slog.Warn("⚠️ IsHoliday: using cached holiday data", "reason", reason)
	return isHolidayInCache(cache)
}
