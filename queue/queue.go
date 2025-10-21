package queue

import (
	"Twilight/redis_client"
	"Twilight/utils"
	"Twilight/yt"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
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
	isPaused    bool                       // True if playback is paused
	mu          sync.Mutex                 // Mutex to protect concurrent access
	stop        chan struct{}              // Channel to signal stopping the session
	resume      chan struct{}              // Channel to signal resuming from pause
	stopped     bool                       // True if session has been stopped already
}

// Pause sets the audio session to paused, stopping audio playback temporarily
func (s *AudioSession) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isPaused {
		s.isPaused = true
		s.resume = make(chan struct{})
	}
}

// IsPaused returns true if the audio session is currently paused
func (s *AudioSession) IsPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isPaused
}

// Resume unpauses the audio session, allowing playback to continue
func (s *AudioSession) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isPaused {
		close(s.resume)
		s.isPaused = false
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
		frameDuration    = 20 * time.Millisecond
	)

	if !vc.Ready {
		for range 20 {
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

	pcmBuffer := make([]int16, frameSize*channels)
	stop := make(chan struct{})

	session.mu.Lock()
	session.VC = vc
	session.Cmd = cmd
	session.Encoder = encoder
	session.isPaused = false
	session.stop = stop
	session.stopped = false
	session.mu.Unlock()

	defer session.Stop()

	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	for {
		session.mu.Lock()
		if session.isPaused {
			resume := session.resume
			session.mu.Unlock()
			ticker.Stop()
			select {
			case <-resume:
				ticker = time.NewTicker(frameDuration)
			case <-session.stop:
				return nil
			}
			continue
		}
		session.mu.Unlock()

		err := binary.Read(stdout, binary.LittleEndian, pcmBuffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		opusFrame, err := encoder.Encode(pcmBuffer, frameSize, maxOpusFrameSize)
		if err != nil {
			return err
		}

		<-ticker.C

		if len(opusFrame) > 0 {
			select {
			case vc.OpusSend <- opusFrame:
			case <-time.After(200 * time.Millisecond):
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

// GuildManager manages all guild queues and sessions
type GuildManager struct {
	mu       sync.RWMutex
	songs    map[string]*QueueData   // Maps guild ID to its queue data
	sessions map[string]*SessionData // Maps guild ID to its audio session
}

var guildManager = &GuildManager{
	songs:    make(map[string]*QueueData),
	sessions: make(map[string]*SessionData),
}

// GetOrCreateQueue returns QueueData if exists otherwise initializes one
func (gm *GuildManager) GetOrCreateQueue(guildID string) *QueueData {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	qd, exists := gm.songs[guildID]
	if !exists {
		qd = &QueueData{Songs: []*QueueSong{}}
		gm.songs[guildID] = qd
	}
	return qd
}

// GetOrCreateSession returns SessionData if exists otherwise initializes one
func (gm *GuildManager) GetOrCreateSession(guildID string) *SessionData {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	sd, exists := gm.sessions[guildID]
	if !exists {
		sd = &SessionData{Session: &AudioSession{}}
		gm.sessions[guildID] = sd
	}
	return sd
}

// GetQueue returns QueueData if exists
func (gm *GuildManager) GetQueue(guildID string) (*QueueData, bool) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	qd, exists := gm.songs[guildID]
	return qd, exists
}

// GetSession returns SessionData if exists
func (gm *GuildManager) GetSession(guildID string) (*SessionData, bool) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	sd, exists := gm.sessions[guildID]
	return sd, exists
}

// DeleteGuild deletes guildID from GuildManager
func (gm *GuildManager) DeleteGuild(guildID string) {
	gm.mu.Lock()
	sd, exists := gm.sessions[guildID]
	if exists {
		delete(gm.sessions, guildID)
	}
	delete(gm.songs, guildID)
	gm.mu.Unlock()

	if exists {
		sd.mu.Lock()
		if sd.Session != nil && !sd.Session.stopped {
			sd.Session.Stop()
		}
		sd.mu.Unlock()
	}
}

// StopAll stops all sessions from GuildManager
func (gm *GuildManager) StopAll() {
	gm.mu.Lock()
	for _, sd := range gm.sessions {
		sd.mu.Lock()
		if sd.Session != nil && !sd.Session.stopped {
			sd.Session.Stop()
		}
		sd.mu.Unlock()
	}
	gm.songs = make(map[string]*QueueData)
	gm.sessions = make(map[string]*SessionData)
	gm.mu.Unlock()
}

// ShuffleGuildQueue shuffles the song queue for a given guild
func ShuffleGuildQueue(guildID string) error {
	qd, exists := guildManager.GetQueue(guildID)
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

// LoopGuildQueue toggles loop for the song queue for a given guild
func LoopGuildQueue(guildID string) (bool, error) {
	qd, exists := guildManager.GetQueue(guildID)
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
	qd := guildManager.GetOrCreateQueue(guildID)
	sd := guildManager.GetOrCreateSession(guildID)

	qd.mu.Lock()
	qd.Songs = append(qd.Songs, &QueueSong{
		Filename:    filename,
		RequestedBy: username,
	})
	songsCopy := qd.Songs
	currentCopy := qd.CurrentSong
	qd.mu.Unlock()

	sd.mu.Lock()
	if sd.Session.stopped {
		sd.Session = &AudioSession{}
	}
	sd.mu.Unlock()

	return &GuildQueue{
		Songs:       songsCopy,
		CurrentSong: currentCopy,
		Loop:        qd.Loop,
		Session:     sd.Session,
	}
}

// playNext plays the next song in the guilds song queue
func PlayNext(s *discordgo.Session, guildID string, vc *discordgo.VoiceConnection) {
	qd, qExists := guildManager.GetQueue(guildID)
	sd, sExists := guildManager.GetSession(guildID)
	if !qExists || !sExists {
		return
	}
	ytManager := yt.NewYouTubeManager(redis_client.RDB)

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

		videoID := utils.GetAudioID(item.Filename)
		if _, err := os.Stat(item.Filename); os.IsNotExist(err) {
			ytManager.DownloadAudio(videoID)
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
	qd, qExists := guildManager.GetQueue(guildID)
	sd, sExists := guildManager.GetSession(guildID)
	if !qExists || !sExists {
		return nil, false
	}

	qd.mu.Lock()
	songsCopy := make([]*QueueSong, len(qd.Songs))
	copy(songsCopy, qd.Songs)

	var currentCopy *QueueSong
	if qd.CurrentSong != nil {
		currentCopy = qd.CurrentSong
	}
	qd.mu.Unlock()

	return &GuildQueue{
		Songs:       songsCopy,
		CurrentSong: currentCopy,
		Loop:        qd.Loop,
		Session:     sd.Session,
	}, true
}

// DeleteGuildQueue removes the guild from guildSongs and guildSessions
func DeleteGuildQueue(guildID string) {
	guildManager.DeleteGuild(guildID)
}

// ClearGuildQueue stops the current song and clears all queued songs for a guild
func ClearGuildQueue(guildID string) {
	qd, qExists := guildManager.GetQueue(guildID)
	sd, sExists := guildManager.GetSession(guildID)
	if !qExists || !sExists {
		return
	}

	// Stop the session
	sd.mu.Lock()
	if sd.Session != nil && !sd.Session.stopped {
		sd.Session.Stop()
	}
	sd.mu.Unlock()

	// Clear queue
	qd.mu.Lock()
	qd.Songs = []*QueueSong{}
	qd.CurrentSong = nil
	qd.mu.Unlock()
}

// ClearCurrentSong clears the currently playing item for a guild
func ClearCurrentSong(guildID string) {
	qd, exists := guildManager.GetQueue(guildID)
	if !exists {
		return
	}

	qd.mu.Lock()
	qd.CurrentSong = nil
	qd.mu.Unlock()
}

// StopAllSessions clears data for all guilds and closes all Sessions
func StopAllSessions() {
	guildManager.StopAll()
}
