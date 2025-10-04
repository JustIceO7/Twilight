package yt

import (
	"Twilight/redis_client"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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

	err := youTubeDownload(videoID, filename)
	if err == nil {
		return nil
	}

	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	err = cmd.Run()
	if err != nil {
		return errors.New(stderr.String())
	}

	return nil
}

// youTubeDownload downloads audio from a given videoID directly to a file using YouTube client
func youTubeDownload(videoID, filename string) error {
	client := youtube.Client{}
	video, err := client.GetVideo("https://www.youtube.com/watch?v=" + videoID)
	if err != nil {
		return err
	}

	formats := video.Formats.WithAudioChannels()

	stream, _, err := client.GetStream(video, &formats[0])
	if err != nil {
		return err
	}
	defer stream.Close()

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	if err != nil {
		os.Remove(filename)
		return err
	}

	return nil
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
