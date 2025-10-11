package playlist

import (
	"Twilight/utils"
	"Twilight/yt"
	"fmt"
	"sync"
)

// DownloadVideosConcurrently downloads a list of video IDs with limited concurrency
func DownloadVideosConcurrently(videoIDs []string, ytManager *yt.YouTubeManager, maxConcurrency int) ([]string, int) {
	successCount := 0
	orderedFilenames := make([]string, len(videoIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	concurrencySem := make(chan struct{}, maxConcurrency)

	// Loops over each videoID and runs go-routines to download concurrently
	for idx, videoID := range videoIDs {
		wg.Add(1)
		go func(index int, vid string) {
			defer wg.Done()

			concurrencySem <- struct{}{}
			defer func() { <-concurrencySem }()

			if err := ytManager.DownloadAudio(vid); err != nil {
				fmt.Printf("DEBUG: Download error for %s: %v\n", vid, err)
				return
			}

			mu.Lock()
			orderedFilenames[index] = utils.GetAudioFile(vid)
			successCount++
			mu.Unlock()
		}(idx, videoID)
	}

	wg.Wait()

	// Filter out failed downloads
	var filenames []string
	for _, fn := range orderedFilenames {
		if fn != "" {
			filenames = append(filenames, fn)
		}
	}

	return filenames, successCount
}
