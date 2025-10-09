package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAudioFile(t *testing.T) {
	videoID := "abc123"
	expected := "cache/abc123.opus"
	assert.Equal(t, expected, GetAudioFile(videoID))
}

func TestGetAudioID(t *testing.T) {
	filepath := "cache/abc123.opus"
	expected := "abc123"
	assert.Equal(t, expected, GetAudioID(filepath))
}
