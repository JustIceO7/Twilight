package yt

import (
	"Twilight/redis_client"
	"Twilight/utils"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/kkdai/youtube/v2"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

type YouTubeManager struct {
	redis        *redis.Client
	cacheYoutube time.Duration
	cacheAudio   time.Duration
}

// NewYouTubeManager creates a YouTubeManager with Redis cache
func NewYouTubeManager(rdb *redis.Client) *YouTubeManager {
	Yt := time.Duration(viper.GetInt("cache.youtube")) * time.Second
	Audio := time.Duration(viper.GetInt("cache.audio")) * time.Second
	return &YouTubeManager{
		redis:        rdb,
		cacheYoutube: Yt,
		cacheAudio:   Audio,
	}
}

// GetVideoMetadata fetches YouTube video metadata given videoID
func (ym *YouTubeManager) GetVideoMetadata(videoID string) (*youtube.Video, error) {
	// Try Redis
	cached, err := ym.redis.Get(redis_client.Ctx, "ytmeta:"+videoID).Result()
	if err == nil && cached != "" {
		var video youtube.Video
		json.Unmarshal([]byte(cached), &video)
		return &video, nil
	}

	// Fetch from Youtube
	video, err := FetchVideoMetadata(videoID)
	if err != nil {
		return nil, err
	}

	// Store in Redis
	data, _ := json.Marshal(video)
	ym.redis.Set(redis_client.Ctx, "ytmeta:"+videoID, data, ym.cacheYoutube)

	return video, nil
}

// DownloadAudio caches and downloads YouTube audio given videoID
func (ym *YouTubeManager) DownloadAudio(videoID string) error {
	ym.redis.Set(redis_client.Ctx, "ytvideo:"+videoID, true, ym.cacheAudio)
	filename := utils.GetAudioFile(videoID)

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		if err := DownloadAudioFile(videoID); err != nil {
			return err
		}
	}

	return nil
}

// GetPlaylistVideoIDs returns all video IDs from a YouTube playlist URL
func (ym *YouTubeManager) GetPlaylistVideoIDs(playlistURL string) ([]string, error) {
	cmd := exec.Command("yt-dlp", "-j", "--flat-playlist", playlistURL)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := bytes.Split(out, []byte("\n"))
	videoIDs := []string{}
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var entry struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		videoIDs = append(videoIDs, entry.ID)
	}

	return videoIDs, nil
}
