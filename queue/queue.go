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

func (s *AudioSession) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsPaused = true
}

func (s *AudioSession) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsPaused = false
}

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

type GuildQueue struct {
	Items       []*QueueItem
	CurrentItem *QueueItem
	Session     *AudioSession
	mu          sync.Mutex
}

var guildQueues = make(map[string]*GuildQueue)

func Enqueue(guildID, filename, username string) *GuildQueue {
	gq, exists := guildQueues[guildID]
	if !exists {
		gq = &GuildQueue{
			Items:   []*QueueItem{},
			Session: &AudioSession{},
		}
		guildQueues[guildID] = gq
	}

	gq.mu.Lock()
	defer gq.mu.Unlock()

	if gq.Session.stopped {
		gq.Session = &AudioSession{}
	}

	gq.Items = append(gq.Items, &QueueItem{
		Filename:    filename,
		RequestedBy: username,
	})
	return gq
}

// playNext plays the next song in the guilds music queue
func PlayNext(s *discordgo.Session, guildID string, vc *discordgo.VoiceConnection) {
	gq, exists := guildQueues[guildID]
	if !exists {
		return
	}

	for {
		gq.mu.Lock()
		if len(gq.Items) == 0 {
			gq.CurrentItem = nil // Clear current item when queue is empty
			gq.mu.Unlock()
			break
		}

		item := gq.Items[0]
		gq.Items = gq.Items[1:]
		gq.CurrentItem = item

		// Reuse the existing session if possible
		if gq.Session == nil || gq.Session.stopped {
			gq.Session = &AudioSession{}
		}

		// Update the VC for the session
		gq.Session.mu.Lock()
		gq.Session.VC = vc
		gq.Session.mu.Unlock()

		session := gq.Session
		gq.mu.Unlock()

		err := playAudioFile(vc, item.Filename, session)
		if err != nil && err.Error() != "EOF" {
			fmt.Printf("Playback error: %v\n", err)
		}

		// Clear current item after song finishes
		gq.mu.Lock()
		gq.CurrentItem = nil
		gq.mu.Unlock()
	}
}

func GetGuildQueue(guildID string) (*GuildQueue, bool) {
	gq, exists := guildQueues[guildID]
	return gq, exists
}

func DeleteGuildQueue(guildID string) {
	delete(guildQueues, guildID)
}

// ClearCurrentItem clears the currently playing item for a guild
func ClearCurrentItem(guildID string) {
	gq, exists := guildQueues[guildID]
	if !exists {
		return
	}

	gq.mu.Lock()
	gq.CurrentItem = nil
	gq.mu.Unlock()
}
