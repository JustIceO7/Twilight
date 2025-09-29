package utils

import (
	"fmt"
	"time"
)

// FormatYtDuration takes in time.Duration and formats in HH:MM:SS
func FormatYtDuration(d time.Duration) string {
	totalSeconds := int(d.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}
