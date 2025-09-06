package yt

import (
	"Twilight/redis_client"
	"encoding/json"
	"io"
	"time"

	"github.com/kkdai/youtube/v2"
)

// FetchVideoStream fetches the audio from a given videoID and returns a stream
func FetchVideoStream(videoID string) (io.ReadCloser, error) {
	client := youtube.Client{}

	video, err := client.GetVideo(videoID)
	if err != nil {
		return nil, err
	}

	formats := video.Formats.WithAudioChannels()
	stream, _, err := client.GetStream(video, &formats[0])
	if err != nil {
		return nil, err
	}

	return stream, nil
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
