package utils

import (
	"fmt"
	"time"
)

// FormatTimezoneOffset formats a timezone offset in seconds to a string like "+08:00" or "-05:30".
// This handles timezones with non-hour offsets (e.g., India +05:30, Nepal +05:45).
func FormatTimezoneOffset(offsetSeconds int) string {
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60
	if minutes > 0 {
		return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
	}
	return fmt.Sprintf("%s%02d", sign, hours)
}

// CurrentTimezoneOffset returns the current timezone offset string.
func CurrentTimezoneOffset() string {
	_, offset := time.Now().Zone()
	return FormatTimezoneOffset(offset)
}

// TodayDate returns today's date in YYYY-MM-DD format.
func TodayDate() string {
	return time.Now().Format("2006-01-02")
}
