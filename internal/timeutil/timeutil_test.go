package timeutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFormatRFC3339_TimezoneConversion tests that the same instant in time
// is correctly converted to different timezone representations.
func TestFormatRFC3339_TimezoneConversion(t *testing.T) {
	// Use a fixed UTC time for testing
	// 2024-01-15 12:30:45 UTC
	utcTime := time.Date(2024, 1, 15, 12, 30, 45, 0, time.UTC)

	tests := []struct {
		name     string
		tz       string
		expected string
	}{
		{
			name:     "UTC timezone",
			tz:       "UTC",
			expected: "2024-01-15T12:30:45Z",
		},
		{
			name:     "Asia/Tokyo (+09:00)",
			tz:       "Asia/Tokyo",
			expected: "2024-01-15T21:30:45+09:00",
		},
		{
			name:     "America/New_York (-05:00 in January)",
			tz:       "America/New_York",
			expected: "2024-01-15T07:30:45-05:00",
		},
		{
			name:     "Europe/London (UTC in January)",
			tz:       "Europe/London",
			expected: "2024-01-15T12:30:45Z",
		},
		{
			name:     "Pacific/Auckland (+13:00 in January)",
			tz:       "Pacific/Auckland",
			expected: "2024-01-16T01:30:45+13:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cache before each test
			ResetLocationCache()

			// t.Setenv automatically handles cleanup
			t.Setenv("TZ", tt.tz)

			result := FormatRFC3339(utcTime)
			assert.Equal(t, tt.expected, result)

			// Verify that parsing the result back gives the same instant
			parsed, err := time.Parse(time.RFC3339, result)
			require.NoError(t, err)
			assert.True(t, utcTime.Equal(parsed),
				"Parsed time should represent the same instant: got %v, want %v",
				parsed.UTC(), utcTime)
		})
	}
}

// TestFormatRFC3339_InvalidTZ tests that invalid TZ falls back to UTC.
func TestFormatRFC3339_InvalidTZ(t *testing.T) {
	ResetLocationCache()

	utcTime := time.Date(2024, 1, 15, 12, 30, 45, 0, time.UTC)

	t.Setenv("TZ", "Invalid/Timezone")

	result := FormatRFC3339(utcTime)
	// Should fall back to UTC
	assert.Equal(t, "2024-01-15T12:30:45Z", result)
}

// TestFormatRFC3339_EmptyTZ tests that empty TZ uses local time.
func TestFormatRFC3339_EmptyTZ(t *testing.T) {
	ResetLocationCache()

	utcTime := time.Date(2024, 1, 15, 12, 30, 45, 0, time.UTC)

	// Unset TZ to test local time behavior
	// Note: Setting TZ="" effectively unsets it (uses local time)
	t.Setenv("TZ", "")

	result := FormatRFC3339(utcTime)

	// Verify it's valid RFC3339 and represents the same instant
	parsed, err := time.Parse(time.RFC3339, result)
	require.NoError(t, err)
	assert.True(t, utcTime.Equal(parsed),
		"Parsed time should represent the same instant")

	// Verify it's using local timezone (the offset should match local)
	expected := utcTime.In(time.Local).Format(time.RFC3339)
	assert.Equal(t, expected, result)
}

// TestFormatRFC3339_PreservesInstant verifies that the formatted string
// always represents the exact same instant in time.
func TestFormatRFC3339_PreservesInstant(t *testing.T) {
	timezones := []string{
		"UTC",
		"Asia/Tokyo",
		"America/Los_Angeles",
		"Europe/Paris",
		"Australia/Sydney",
	}

	// Test with various times including edge cases
	testTimes := []time.Time{
		time.Date(2024, 1, 15, 12, 30, 45, 0, time.UTC),
		time.Date(2024, 6, 15, 12, 30, 45, 0, time.UTC),  // DST in some zones
		time.Date(2024, 3, 10, 10, 30, 0, 0, time.UTC),   // DST transition day in US
		time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC), // Year boundary
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),      // Year start
	}

	for _, tz := range timezones {
		for _, testTime := range testTimes {
			t.Run(tz+"_"+testTime.Format("2006-01-02T15:04:05"), func(t *testing.T) {
				ResetLocationCache()
				t.Setenv("TZ", tz)

				result := FormatRFC3339(testTime)

				// Parse it back
				parsed, err := time.Parse(time.RFC3339, result)
				require.NoError(t, err)

				// Unix timestamps must match exactly
				assert.Equal(t, testTime.Unix(), parsed.Unix(),
					"Unix timestamp must be preserved for TZ=%s, time=%v",
					tz, testTime)

				// Also check with Equal which compares the instant
				assert.True(t, testTime.Equal(parsed),
					"Times must represent the same instant for TZ=%s", tz)
			})
		}
	}
}

// TestGetLocation_Caching verifies that the location is cached.
func TestGetLocation_Caching(t *testing.T) {
	ResetLocationCache()

	t.Setenv("TZ", "Asia/Tokyo")

	// First call loads the location
	loc1 := GetLocation()
	assert.Equal(t, "Asia/Tokyo", loc1.String())

	// Change TZ - should not affect cached value
	t.Setenv("TZ", "America/New_York")

	// Second call should return cached value
	loc2 := GetLocation()
	assert.Equal(t, "Asia/Tokyo", loc2.String())

	// Verify they're the same pointer (cached)
	assert.Same(t, loc1, loc2)
}

// TestLoadLocation_DirectCalls tests loadLocation without caching.
func TestLoadLocation_DirectCalls(t *testing.T) {
	t.Run("valid timezone", func(t *testing.T) {
		t.Setenv("TZ", "Europe/Berlin")
		loc := loadLocation()
		assert.Equal(t, "Europe/Berlin", loc.String())
	})

	t.Run("empty TZ uses local", func(t *testing.T) {
		// Unset TZ - setting TZ="" effectively unsets it (uses local time)
		t.Setenv("TZ", "")
		loc := loadLocation()
		assert.Equal(t, time.Local, loc)
	})

	t.Run("invalid TZ falls back to UTC", func(t *testing.T) {
		t.Setenv("TZ", "Not/A/Real/Timezone")
		loc := loadLocation()
		assert.Equal(t, time.UTC, loc)
	})
}
