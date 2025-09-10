package utils

import (
	"fmt"
	"time"
)

// FormatYtDuration takes in time.Duration and formats in DD:HH:MM
func FormatYtDuration(d time.Duration) string {
	totalMinutes := int(d.Minutes())
	days := totalMinutes / (24 * 60)
	hours := (totalMinutes % (24 * 60)) / 60
	minutes := totalMinutes % 60
	return fmt.Sprintf("%02d:%02d:%02d", days, hours, minutes)
}
