package yt

import (
	"Twilight/redis_client"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/kkdai/youtube/v2"
)

// DownloadVideo downloads the audio from a given videoID directly to a file
func DownloadVideo(videoID string) error {
	filename := fmt.Sprintf("cache/%s.opus", videoID)
	cmd := exec.Command("yt-dlp",
		"-f", "bestaudio[ext=opus]/bestaudio",
		"-o", filename,
		"https://www.youtube.com/watch?v="+videoID,
	)
	redis_client.RDB.Set(redis_client.Ctx, "video:"+videoID, true, 3600*time.Second) // 1 hour TTL
	return cmd.Run()
}

// FetchVideoMetadata fetches basic metadata for a given videoID
func FetchVideoMetadata(videoID string) (*youtube.Video, error) {
	// Try Redis
	cached, err := redis_client.RDB.Get(redis_client.Ctx, "ytmeta:"+videoID).Result()
	if err == nil && cached != "" {
		var video youtube.Video
		json.Unmarshal([]byte(cached), &video)
		return &video, nil
	}

	// Fetch from Youtube
	client := youtube.Client{}
	video, err := client.GetVideo(videoID)
	if err != nil {
		return nil, err
	}

	// Store in Redis
	data, _ := json.Marshal(video)
	redis_client.RDB.Set(redis_client.Ctx, "ytmeta:"+videoID, data, 3600*time.Second) // 1 hour TTL

	return video, nil
}
