// Package timeutil provides timezone-aware time formatting utilities.
package timeutil

import (
	"os"
	"sync"
	"time"
)

//nolint:gochecknoglobals // cached timezone location for performance
var (
	// locationCache caches the loaded timezone location.
	locationCache *time.Location
	locationOnce  sync.Once
)

// loadLocation loads the timezone from TZ environment variable.
// Falls back to UTC if TZ is invalid.
// Uses local time if TZ is not set.
func loadLocation() *time.Location {
	tz := os.Getenv("TZ")
	if tz == "" {
		// TZ not set: use system local timezone
		return time.Local
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		// TZ is invalid: fall back to UTC for safety
		return time.UTC
	}

	return loc
}

// GetLocation returns the timezone location based on TZ environment variable.
// The result is cached after the first call.
func GetLocation() *time.Location {
	locationOnce.Do(func() {
		locationCache = loadLocation()
	})

	return locationCache
}

// FormatRFC3339 formats the given time in RFC3339 format using the
// timezone from TZ environment variable.
func FormatRFC3339(t time.Time) string {
	return t.In(GetLocation()).Format(time.RFC3339)
}

// ResetLocationCache resets the cached location.
// This is intended for testing purposes only.
func ResetLocationCache() {
	locationOnce = sync.Once{}
	locationCache = nil
}
