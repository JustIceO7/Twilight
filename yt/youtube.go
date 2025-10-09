package yt

import (
	"Twilight/utils"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"

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

// FetchVideoMetadata fetches basic metadata for a given videoID
func FetchVideoMetadata(videoID string) (*youtube.Video, error) {
	// Fetch from Youtube
	client := youtube.Client{}
	video, err := client.GetVideo(videoID)
	if err != nil {
		return nil, err
	}

	return video, nil
}
