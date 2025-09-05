package queue

import (
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
		"-ar", "48000",
		"-ac", "2",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	encoder, err := gopus.NewEncoder(48000, 2, gopus.Audio)
	if err != nil {
		cmd.Process.Kill()
		return err
	}

	pcmBuffer := make([]byte, 3840)
	int16Buffer := make([]int16, 1920*2)
	stop := make(chan struct{})

	session.mu.Lock()
	session.VC = vc
	session.Cmd = cmd
	session.Encoder = encoder
	session.PcmBuffer = pcmBuffer
	session.Int16Buffer = int16Buffer
	session.IsPaused = false
	session.stop = stop
	session.stopped = false
	session.mu.Unlock()

	defer session.Stop()

	pcmCache := []int16{}

	for {
		select {
		case <-stop:
			return nil
		default:
		}

		session.mu.Lock()
		paused := session.IsPaused
		session.mu.Unlock()

		if paused {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		n, err := stdout.Read(pcmBuffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		for i := 0; i < n; i += 2 {
			if i+1 < n {
				sample := int16(pcmBuffer[i]) | int16(pcmBuffer[i+1])<<8
				pcmCache = append(pcmCache, sample)
			}
		}

		for len(pcmCache) >= 960*2 { // 960 samples per channel, 2 channels
			frame := pcmCache[:960*2]
			pcmCache = pcmCache[960*2:]

			opusFrame, err := encoder.Encode(frame, 960, 4000)
			if err != nil {
				return err
			}

			if len(opusFrame) > 0 {
				select {
				case vc.OpusSend <- opusFrame:
				case <-time.After(100 * time.Millisecond):
					return fmt.Errorf("timeout sending opus frame")
				case <-stop:
					return nil
				}
			}
		}
	}

	return cmd.Wait()
}

type QueueItem struct {
	Filename    string // Path to the audio file
	RequestedBy string // Username of who requested the song
}

var guildItems = make(map[string]*QueueData)      // Maps guild ID to its queue data
var guildSessions = make(map[string]*SessionData) // Maps guild ID to its audio session

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
