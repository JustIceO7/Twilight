package queue

import (
	"Twilight/redis_client"
	"Twilight/yt"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
)

type AudioSession struct {
	VC          *discordgo.VoiceConnection // Discord voice connection for this session
	Cmd         *exec.Cmd                  // ffmpeg process converting audio to PCM
	Encoder     *gopus.Encoder             // Opus encoder for sending audio to Discord
	PcmBuffer   []byte                     // Buffer for raw audio bytes from ffmpeg
	Int16Buffer []int16                    // Buffer for PCM audio as 16-bit samples
	IsPaused    bool                       // True if playback is paused
	mu          sync.Mutex                 // Mutex to protect concurrent access
	stop        chan struct{}              // Channel to signal stopping the session
	resume      chan struct{}              // Channel to signal resuming from pause
	stopped     bool                       // True if session has been stopped already
}

// Pause sets the audio session to paused, stopping audio playback temporarily
func (s *AudioSession) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.IsPaused {
		s.IsPaused = true
		s.resume = make(chan struct{})
	}
}

// Resume unpauses the audio session, allowing playback to continue
func (s *AudioSession) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.IsPaused {
		close(s.resume)
		s.IsPaused = false
		s.resume = nil
	}
}

// Stop completely stops the audio session, kills ffmpeg, clears buffers, and ends playback
func (s *AudioSession) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}
	s.stopped = true

	if s.stop != nil {
		close(s.stop)
	}
	if s.Cmd != nil && s.Cmd.Process != nil {
		s.Cmd.Process.Kill()
		s.Cmd.Wait()
	}
	if s.VC != nil {
		s.VC.Speaking(false)
	}

	s.PcmBuffer = nil
	s.Int16Buffer = nil
	s.Encoder = nil
}

// playAudioFile streams audio to Discord
func playAudioFile(vc *discordgo.VoiceConnection, filename string, session *AudioSession) error {
	const (
		sampleRate       = 48000
		channels         = 2
		frameSize        = 960
		maxOpusFrameSize = 4000
	)

	if !vc.Ready {
		for i := 0; i < 20; i++ {
			time.Sleep(100 * time.Millisecond)
			if vc.Ready {
				break
			}
		}
		if !vc.Ready {
			return fmt.Errorf("voice connection never became ready")
		}
	}

	vc.Speaking(true)
	defer vc.Speaking(false)

	cmd := exec.Command("ffmpeg",
		"-i", filename,
		"-f", "s16le",
		"-ar", fmt.Sprintf("%d", sampleRate),
		"-ac", fmt.Sprintf("%d", channels),
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	encoder, err := gopus.NewEncoder(sampleRate, channels, gopus.Audio)
	if err != nil {
		cmd.Process.Kill()
		return err
	}

	buf := make([]int16, frameSize*channels)
	stop := make(chan struct{})

	session.mu.Lock()
	session.VC = vc
	session.Cmd = cmd
	session.Encoder = encoder
	session.IsPaused = false
	session.stop = stop
	session.stopped = false
	session.mu.Unlock()

	defer session.Stop()

	for {
		session.mu.Lock()
		if session.IsPaused {
			resume := session.resume
			session.mu.Unlock()
			select {
			case <-resume:
			case <-session.stop:
				return nil
			}
			continue
		}
		session.mu.Unlock()

		err := binary.Read(stdout, binary.LittleEndian, buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		opus, err := encoder.Encode(buf, frameSize, maxOpusFrameSize)
		if err != nil {
			return err
		}

		if len(opus) > 0 {
			select {
			case vc.OpusSend <- opus:
			case <-time.After(50 * time.Millisecond):
				return fmt.Errorf("timeout sending opus frame")
			case <-stop:
				return nil
			}
		}
	}

	return cmd.Wait()
}

type QueueSong struct {
	Filename    string // Path to the audio file
	RequestedBy string // Username of who requested the song
}

var (
	guildSongs    = make(map[string]*QueueData)   // Maps guild ID to its queue data
	guildSessions = make(map[string]*SessionData) // Maps guild ID to its audio session
)

type QueueData struct {
	Songs       []*QueueSong // List of queued songs
	CurrentSong *QueueSong   // Currently playing song
	Loop        bool         // Queue Loop
	mu          sync.Mutex   // Mutex to protect concurrent access
}

type SessionData struct {
	Session *AudioSession // Audio session for this guild
	mu      sync.Mutex    // Mutex to protect concurrent access
}

type GuildQueue struct {
	Songs       []*QueueSong  // Copy of queued songs
	CurrentSong *QueueSong    // Copy of currently playing song
	Loop        bool          // Queue Loop
	Session     *AudioSession // Copy of the current audio session
	mu          sync.Mutex    // Mutex to protect concurrent access
}

// ShuffleGuildQueue shuffles the song queue for a given guild
func ShuffleGuildQueue(guildID string) error {
	qd, exists := guildSongs[guildID]
	if !exists {
		return fmt.Errorf("no queue for guild %s", guildID)
	}

	qd.mu.Lock()
	defer qd.mu.Unlock()

	rand.Shuffle(len(qd.Songs), func(i, j int) {
		qd.Songs[i], qd.Songs[j] = qd.Songs[j], qd.Songs[i]
	})
	return nil
}

func LoopGuildQueue(guildID string) (bool, error) {
	qd, exists := guildSongs[guildID]
	if !exists {
		return false, fmt.Errorf("no queue for guild %s", guildID)
	}

	qd.mu.Lock()
	defer qd.mu.Unlock()
	qd.Loop = !qd.Loop
	return qd.Loop, nil
}

// Enqueue queues a song into the queue for a given guild
func Enqueue(guildID, filename, username string) *GuildQueue {
	qd, exists := guildSongs[guildID]
	if !exists {
		qd = &QueueData{Songs: []*QueueSong{}}
		guildSongs[guildID] = qd
	}

	// Ensure SessionData exists
	sd, exists := guildSessions[guildID]
	if !exists {
		sd = &SessionData{Session: &AudioSession{}}
		guildSessions[guildID] = sd
	}

	qd.mu.Lock()
	defer qd.mu.Unlock()

	sd.mu.Lock()
	if sd.Session.stopped {
		sd.Session = &AudioSession{}
	}
	sd.mu.Unlock()

	qd.Songs = append(qd.Songs, &QueueSong{
		Filename:    filename,
		RequestedBy: username,
	})

	return &GuildQueue{
		Songs:       qd.Songs,
		CurrentSong: qd.CurrentSong,
		Session:     sd.Session,
	}
}

// playNext plays the next song in the guilds song queue
func PlayNext(s *discordgo.Session, guildID string, vc *discordgo.VoiceConnection) {
	qd, exists := guildSongs[guildID]
	if !exists {
		return
	}
	sd, exists := guildSessions[guildID]
	if !exists {
		return
	}

	for {
		qd.mu.Lock()
		if len(qd.Songs) == 0 {
			qd.CurrentSong = nil // Clear current item when queue is empty
			qd.mu.Unlock()
			break
		}

		item := qd.Songs[0]
		qd.Songs = qd.Songs[1:]
		qd.CurrentSong = item
		qd.mu.Unlock()

		sd.mu.Lock()
		if sd.Session == nil || sd.Session.stopped {
			sd.Session = &AudioSession{}
		}
		sd.Session.VC = vc
		session := sd.Session
		sd.mu.Unlock()

		videoID := strings.TrimSuffix(strings.TrimPrefix(item.Filename, "cache/"), ".mp3")
		if _, err := os.Stat(item.Filename); os.IsNotExist(err) {
			yt.DownloadVideo(videoID)
		} else {
			redis_client.RDB.Set(redis_client.Ctx, "video:"+videoID, true, 3600*time.Second) // 1 hour TTL
		}

		err := playAudioFile(vc, item.Filename, session)
		if err != nil && err.Error() != "EOF" && err.Error() != "unexpected EOF" {
			fmt.Printf("Playback error: %v\n", err)
		}

		qd.mu.Lock()
		if qd.Loop {
			qd.Songs = append(qd.Songs, item)
		} else {
			qd.CurrentSong = nil
		}
		qd.mu.Unlock()
	}
}

// GetGuildQueue returns the full queue for a given guild
func GetGuildQueue(guildID string) (*GuildQueue, bool) {
	qd, qExists := guildSongs[guildID]
	sd, sExists := guildSessions[guildID]
	if !qExists || !sExists {
		return nil, false
	}

	qd.mu.Lock()
	defer qd.mu.Unlock()
	sd.mu.Lock()
	defer sd.mu.Unlock()

	songsCopy := make([]*QueueSong, len(qd.Songs))
	copy(songsCopy, qd.Songs)

	var currentCopy *QueueSong
	if qd.CurrentSong != nil {
		currentCopy = qd.CurrentSong
	}

	return &GuildQueue{
		Songs:       songsCopy,
		CurrentSong: currentCopy,
		Loop:        qd.Loop,
		Session:     sd.Session,
	}, true
}

// DeleteGuildQueue removes the guild from guildSongs and guildSessions
func DeleteGuildQueue(guildID string) {
	if sd, exists := guildSessions[guildID]; exists {
		sd.mu.Lock()
		if sd.Session != nil {
			sd.Session.Stop()
		}
		sd.mu.Unlock()
		delete(guildSessions, guildID)
	}
	delete(guildSongs, guildID)
}

// ClearCurrentSong clears the currently playing item for a guild
func ClearCurrentSong(guildID string) {
	qd, exists := guildSongs[guildID]
	if !exists {
		return
	}

	qd.mu.Lock()
	qd.CurrentSong = nil
	qd.mu.Unlock()
}

// StopAllSessions clears data for all guilds and closes all Sessions
func StopAllSessions() {
	// Stop all sessions
	for _, sd := range guildSessions {
		if sd != nil {
			sd.mu.Lock()
			if sd.Session != nil && !sd.Session.stopped {
				sd.Session.Stop()
			}
			sd.mu.Unlock()
		}
	}

	// Clear all queue data
	for guildID := range guildSongs {
		delete(guildSongs, guildID)
	}

	// Clear all session data
	for guildID := range guildSessions {
		delete(guildSessions, guildID)
	}
}
