package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type YtDurationTestCase struct {
	input    time.Duration
	expected string
}

func TestFormatYtDuration(t *testing.T) {
	tests := []YtDurationTestCase{
		{0 * time.Second, "00:00:00"},
		{45 * time.Second, "00:00:45"},
		{3*time.Minute + 45*time.Second, "00:03:45"},
		{1*time.Hour + 23*time.Minute + 45*time.Second, "01:23:45"},
		{48*time.Hour + 30*time.Minute + 15*time.Second, "48:30:15"},
	}

	for _, tt := range tests {
		result := FormatYtDuration(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}
