package yt

import (
	"io"

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
	client := youtube.Client{}

	video, err := client.GetVideo(videoID)
	if err != nil {
		return nil, err
	}

	return video, nil
}
