package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"Twilight/yt"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
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
	Items   []*QueueItem
	Session *AudioSession
	mu      sync.Mutex
}

var guildQueues = make(map[string]*GuildQueue)

func enqueue(guildID, filename, username string) *GuildQueue {
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
func playNext(s *discordgo.Session, guildID string, vc *discordgo.VoiceConnection) {
	gq, exists := guildQueues[guildID]
	if !exists {
		return
	}

	for {
		gq.mu.Lock()
		if len(gq.Items) == 0 {
			gq.mu.Unlock()
			break
		}

		item := gq.Items[0]
		gq.Items = gq.Items[1:]

		if gq.Session != nil {
			gq.Session.Stop()
		}

		session := &AudioSession{}
		gq.Session = session
		gq.mu.Unlock()

		err := playAudioFile(vc, item.Filename, session)
		if err != nil && err.Error() != "EOF" {
			fmt.Printf("Playback error: %v\n", err)
		}
	}
}

// playMusic plays the music given a link, adding the music to the music queue
func playMusic(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	// Get user's current voice channel
	vs, err := s.State.VoiceState(i.GuildID, i.Member.User.ID)
	if err != nil || vs == nil || vs.ChannelID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Join a voice channel first 😉",
			},
		})
		return nil
	}

	// Check if bot is already in a voice channel
	if vc, ok := s.VoiceConnections[i.GuildID]; ok && vc != nil {
		if vc.ChannelID != vs.ChannelID {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "I'm already in another voice channel 😅",
				},
			})
			return nil
		}
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Adding your track... 🎶",
		},
	})
	if err != nil {
		return &interactionError{err: err, message: "Failed to respond"}
	}

	vc, err := connectUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if err != nil {
		return nil
	}

	videoURL := i.ApplicationCommandData().Options[0].StringValue()
	videoID, err := youtube.ExtractVideoID(videoURL)
	if err != nil {
		return nil
	}

	os.MkdirAll("cache", 0755)
	filename := fmt.Sprintf("cache/%s.mp3", videoID)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		stream, err := yt.FetchVideoStream(videoID)
		if err != nil {
			return nil
		}

		out, err := os.Create(filename)
		if err != nil {
			stream.Close()
			return nil
		}

		_, err = io.Copy(out, stream)
		out.Close()
		stream.Close()
		if err != nil {
			os.Remove(filename)
			return nil
		}
	}

	gq := enqueue(i.GuildID, filename, i.Member.User.Username)
	if gq.Session.VC == nil {
		go playNext(s, i.GuildID, vc)
	}

	return nil
}

// pauseMusic pauses the current music
func pauseMusic(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := guildQueues[i.GuildID]
	if !ok || gq.Session.VC == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Nothing is playing right now 😶"},
		})
		return nil
	}
	gq.Session.Pause()
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "⏸️ Paused"},
	})
	return nil
}

// resumeMusic resumes the current music
func resumeMusic(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := guildQueues[i.GuildID]
	if !ok || gq.Session.VC == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Nothing is playing right now 😶"},
		})
		return nil
	}
	gq.Session.Resume()
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "▶️ Resumed"},
	})
	return nil
}

// stopMusic stops the current session and disconnects the bot from the voice channel
func stopMusic(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := guildQueues[i.GuildID]
	if !ok {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Nothing is playing right now 😶"},
		})
		return nil
	}

	gq.Session.Stop()
	if gq.Session.VC != nil {
		gq.Session.VC.Disconnect()
	}

	delete(guildQueues, i.GuildID)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "⏹️ Stopped"},
	})
	return nil
}

// skipMusic skips the current music playing and moves on to the next
func skipMusic(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	gq, ok := guildQueues[i.GuildID]
	if !ok || gq.Session.VC == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Nothing is playing right now 😶"},
		})
		return nil
	}

	gq.Session.Stop()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "⏭️ Skipped"},
	})

	return nil
}
