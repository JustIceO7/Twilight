package queue

import (
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
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
	stopped     bool                       // True if session has been stopped already
}

// Pause sets the audio session to paused, stopping audio playback temporarily
func (s *AudioSession) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsPaused = true
}

// Resume unpauses the audio session, allowing playback to continue
func (s *AudioSession) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsPaused = false
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
			time.Sleep(250 * time.Millisecond)
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
			session.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
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
			case <-time.After(100 * time.Millisecond):
				return fmt.Errorf("timeout sending opus frame")
			case <-stop:
				return nil
			}
		}
	}

	return cmd.Wait()
}

type QueueItem struct {
	Filename    string // Path to the audio file
	RequestedBy string // Username of who requested the song
}

var (
	guildItems    = make(map[string]*QueueData)   // Maps guild ID to its queue data
	guildSessions = make(map[string]*SessionData) // Maps guild ID to its audio session
)

type QueueData struct {
	Items       []*QueueItem // List of queued songs
	CurrentItem *QueueItem   // Currently playing song
	mu          sync.Mutex   // Mutex to protect concurrent access
}

type SessionData struct {
	Session *AudioSession // Audio session for this guild
	mu      sync.Mutex    // Mutex to protect concurrent access
}

type GuildQueue struct {
	Items       []*QueueItem  // Copy of queued songs
	CurrentItem *QueueItem    // Copy of currently playing song
	Session     *AudioSession // Copy of the current audio session
	mu          sync.Mutex    // Mutex to protect concurrent access
}

// Enqueue queues a song into the queue for a given guild
func Enqueue(guildID, filename, username string) *GuildQueue {
	qd, exists := guildItems[guildID]
	if !exists {
		qd = &QueueData{Items: []*QueueItem{}}
		guildItems[guildID] = qd
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

	qd.Items = append(qd.Items, &QueueItem{
		Filename:    filename,
		RequestedBy: username,
	})

	return &GuildQueue{
		Items:       qd.Items,
		CurrentItem: qd.CurrentItem,
		Session:     sd.Session,
	}
}

// playNext plays the next song in the guilds music queue
func PlayNext(s *discordgo.Session, guildID string, vc *discordgo.VoiceConnection) {
	qd, exists := guildItems[guildID]
	if !exists {
		return
	}
	sd, exists := guildSessions[guildID]
	if !exists {
		return
	}

	for {
		qd.mu.Lock()
		if len(qd.Items) == 0 {
			qd.CurrentItem = nil // Clear current item when queue is empty
			qd.mu.Unlock()
			break
		}

		item := qd.Items[0]
		qd.Items = qd.Items[1:]
		qd.CurrentItem = item
		qd.mu.Unlock()

		sd.mu.Lock()
		if sd.Session == nil || sd.Session.stopped {
			sd.Session = &AudioSession{}
		}
		sd.Session.VC = vc
		session := sd.Session
		sd.mu.Unlock()

		err := playAudioFile(vc, item.Filename, session)
		if err != nil && err.Error() != "EOF" {
			fmt.Printf("Playback error: %v\n", err)
		}

		qd.mu.Lock()
		qd.CurrentItem = nil
		qd.mu.Unlock()
	}
}

// GetGuildQueue returns the full queue for a given guild
func GetGuildQueue(guildID string) (*GuildQueue, bool) {
	qd, qExists := guildItems[guildID]
	sd, sExists := guildSessions[guildID]
	if !qExists || !sExists {
		return nil, false
	}

	qd.mu.Lock()
	defer qd.mu.Unlock()
	sd.mu.Lock()
	defer sd.mu.Unlock()

	itemsCopy := make([]*QueueItem, len(qd.Items))
	copy(itemsCopy, qd.Items)

	var currentCopy *QueueItem
	if qd.CurrentItem != nil {
		currentCopy = qd.CurrentItem
	}

	return &GuildQueue{
		Items:       itemsCopy,
		CurrentItem: currentCopy,
		Session:     sd.Session,
	}, true
}

// DeleteGuildQueue removes the guild from guildItems and guildSessions
func DeleteGuildQueue(guildID string) {
	if sd, exists := guildSessions[guildID]; exists {
		sd.mu.Lock()
		if sd.Session != nil {
			sd.Session.Stop()
		}
		sd.mu.Unlock()
		delete(guildSessions, guildID)
	}
	delete(guildItems, guildID)
}

// ClearCurrentItem clears the currently playing item for a guild
func ClearCurrentItem(guildID string) {
	qd, exists := guildItems[guildID]
	if !exists {
		return
	}

	qd.mu.Lock()
	qd.CurrentItem = nil
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
	for guildID := range guildItems {
		delete(guildItems, guildID)
	}

	// Clear all session data
	for guildID := range guildSessions {
		delete(guildSessions, guildID)
	}
}
