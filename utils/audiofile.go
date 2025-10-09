package utils

import (
	"fmt"
	"strings"
)

func GetAudioFile(videoID string) string {
	return fmt.Sprintf("cache/%s.opus", videoID)
}

func GetAudioID(filepath string) string {
	return strings.TrimSuffix(strings.TrimPrefix(filepath, "cache/"), ".opus")
}
