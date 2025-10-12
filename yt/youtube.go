package yt

import (
	"Twilight/utils"
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

// DownloadAudioFile downloads the audio from a given videoID directly to a file
func DownloadAudioFile(videoID string) error {
	filename := utils.GetAudioFile(videoID)
	cmd := exec.Command("yt-dlp",
		"-f", "bestaudio/best",
		"-x",
		"--audio-format", "opus",
		"--buffer-size", "16K",
		"-o", filename,
		"https://www.youtube.com/watch?v="+videoID,
	)

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

type Video struct {
	ID          string
	Title       string
	Description string
	Author      string
	Views       int
	Duration    time.Duration
	PublishDate time.Time
	Thumbnail   string
}

// FetchVideoMetadata fetches basic metadata for a given videoID
func FetchVideoMetadata(videoID string) (*Video, error) {
	videoID, _ = youtube.ExtractVideoID(videoID)
	url := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)

	video, err := youTubeMetadata(videoID)
	if err == nil {
		return video, nil
	}

	cmd := exec.Command("yt-dlp", "--dump-single-json", "--skip-download", "--no-playlist", url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp failed: %w", err)
	}

	var data struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Uploader    string `json:"uploader"`
		ViewCount   int    `json:"view_count"`
		Duration    int    `json:"duration"`
		UploadDate  string `json:"upload_date"`
		Thumbnail   string `json:"thumbnail"`
	}

	if err := json.Unmarshal(output, &data); err != nil {
		return nil, err
	}

	publishDate, _ := time.Parse("20060102", data.UploadDate)

	return &Video{
		ID:          data.ID,
		Title:       data.Title,
		Description: data.Description,
		Author:      data.Uploader,
		Views:       data.ViewCount,
		Duration:    time.Duration(data.Duration) * time.Second,
		PublishDate: publishDate,
		Thumbnail:   data.Thumbnail,
	}, nil
}

// youTubeMetadata fetches basic metadata for a given videoID using YouTube client
func youTubeMetadata(videoID string) (*Video, error) {
	client := youtube.Client{}
	v, err := client.GetVideo(videoID)
	if err != nil {
		return nil, err
	}

	return &Video{
		ID:          v.ID,
		Title:       v.Title,
		Author:      v.Author,
		Views:       v.Views,
		Duration:    v.Duration,
		PublishDate: v.PublishDate,
		Thumbnail:   v.Thumbnails[0].URL,
	}, nil
}
