package music

import (
	"io"

	"github.com/kkdai/youtube/v2"
)

// fetchVideoStream fetches the audio from a given videoID and returns a stream
func fetchVideoStream(videoID string) (io.ReadCloser, error) {
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

func fetchVideoMetadata(videoID string) {

}
