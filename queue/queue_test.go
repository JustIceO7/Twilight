package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAudioSession_Pause(t *testing.T) {
	session := &AudioSession{}

	assert.False(t, session.IsPaused())

	session.Pause()

	assert.True(t, session.IsPaused())
	assert.NotNil(t, session.resume)
}

func TestAudioSession_Resume(t *testing.T) {
	session := &AudioSession{}

	session.Pause()

	assert.True(t, session.IsPaused())

	session.Resume()

	assert.False(t, session.IsPaused())
	assert.Nil(t, session.resume)
}

func TestAudioSession_PauseTwice(t *testing.T) {
	session := &AudioSession{}

	session.Pause()
	firstResumeChannel := session.resume

	session.Pause()
	secondResumeChannel := session.resume

	assert.Equal(t, firstResumeChannel, secondResumeChannel)
	assert.True(t, session.IsPaused())
}

func TestAudioSession_ResumeWithoutPause(t *testing.T) {
	session := &AudioSession{}

	session.Resume()

	assert.False(t, session.IsPaused())
}

func TestAudioSession_Stop(t *testing.T) {
	session := &AudioSession{
		stop: make(chan struct{}),
	}

	session.Stop()

	assert.True(t, session.stopped)
	assert.Nil(t, session.PcmBuffer)
	assert.Nil(t, session.Int16Buffer)
	assert.Nil(t, session.Encoder)
}

func TestAudioSession_StopTwice(t *testing.T) {
	session := &AudioSession{
		stop: make(chan struct{}),
	}

	session.Stop()
	session.Stop() // Should not panic or cause issues

	assert.True(t, session.stopped)
}

func TestEnqueue(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	guildID := "test-guild-123"
	filename := "cache/test-song.opus"
	username := "testuser"

	gq := Enqueue(guildID, filename, username)

	assert.NotNil(t, gq)
	assert.Equal(t, 1, len(gq.Songs))
	assert.Equal(t, filename, gq.Songs[0].Filename)
	assert.Equal(t, username, gq.Songs[0].RequestedBy)
	assert.NotNil(t, gq.Session)
}

func TestEnqueue_MultipleSongs(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	guildID := "test-guild-123"

	Enqueue(guildID, "cache/song1.opus", "user1")
	Enqueue(guildID, "cache/song2.opus", "user2")
	gq := Enqueue(guildID, "cache/song3.opus", "user3")

	assert.Equal(t, 3, len(gq.Songs))

	expectedFiles := []string{"cache/song1.opus", "cache/song2.opus", "cache/song3.opus"}
	for i, expected := range expectedFiles {
		assert.Equal(t, expected, gq.Songs[i].Filename)
	}
}

func TestGetGuildQueue(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	guildID := "test-guild-123"

	Enqueue(guildID, "cache/song1.opus", "user1")
	Enqueue(guildID, "cache/song2.opus", "user2")

	gq, exists := GetGuildQueue(guildID)

	assert.True(t, exists)
	assert.NotNil(t, gq)
	assert.Equal(t, 2, len(gq.Songs))
}

func TestGetGuildQueue_NonExistent(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	guildID := "non-existent-guild"

	gq, exists := GetGuildQueue(guildID)

	assert.False(t, exists)
	assert.Nil(t, gq)
}

func TestClearCurrentSong(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	guildID := "test-guild-clear"
	Enqueue(guildID, "cache/song1.opus", "user1")

	qd, _ := guildManager.GetQueue(guildID)
	qd.mu.Lock()
	qd.CurrentSong = &QueueSong{Filename: "cache/current.opus", RequestedBy: "user"}
	qd.mu.Unlock()

	ClearCurrentSong(guildID)

	qd.mu.Lock()
	defer qd.mu.Unlock()

	assert.Nil(t, qd.CurrentSong)
}

func TestClearCurrentSong_NonExistent(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	// Should not panic
	assert.NotPanics(t, func() {
		ClearCurrentSong("non-existent-guild")
	})
}

func TestDeleteGuildQueue(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	guildID := "test-guild-delete"
	Enqueue(guildID, "cache/song1.opus", "user1")

	guildManager.mu.RLock()
	_, exists := guildManager.songs[guildID]
	guildManager.mu.RUnlock()
	assert.True(t, exists)

	DeleteGuildQueue(guildID)

	guildManager.mu.RLock()
	_, exists = guildManager.songs[guildID]
	guildManager.mu.RUnlock()
	assert.False(t, exists)

	guildManager.mu.RLock()
	_, exists = guildManager.sessions[guildID]
	guildManager.mu.RUnlock()
	assert.False(t, exists)
}

func TestStopAllSessions(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	Enqueue("guild1", "cache/song1.opus", "user1")
	Enqueue("guild2", "cache/song2.opus", "user2")
	Enqueue("guild3", "cache/song3.opus", "user3")

	guildManager.mu.RLock()
	songCount := len(guildManager.songs)
	guildManager.mu.RUnlock()
	assert.Equal(t, 3, songCount)

	StopAllSessions()

	guildManager.mu.RLock()
	songCount = len(guildManager.songs)
	sessionCount := len(guildManager.sessions)
	guildManager.mu.RUnlock()
	assert.Equal(t, 0, songCount)
	assert.Equal(t, 0, sessionCount)
}

func TestLoopGuildQueue(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	guildID := "test-guild-loop"
	Enqueue(guildID, "cache/song1.opus", "user1")

	qd, _ := guildManager.GetQueue(guildID)
	qd.mu.Lock()
	initialLoop := qd.Loop
	qd.mu.Unlock()

	loopState, err := LoopGuildQueue(guildID)
	assert.NoError(t, err)
	assert.NotEqual(t, initialLoop, loopState)

	loopState2, err := LoopGuildQueue(guildID)
	assert.NoError(t, err)
	assert.NotEqual(t, loopState, loopState2)
	assert.Equal(t, initialLoop, loopState2)
}

func TestLoopGuildQueue_NonExistent(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	loopState, err := LoopGuildQueue("non-existent-guild")
	assert.Error(t, err)
	assert.False(t, loopState)
}

func TestShuffleGuildQueue(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	guildID := "test-guild-shuffle"

	// Add multiple songs
	for i := 0; i < 10; i++ {
		Enqueue(guildID, "cache/song"+string(rune(i))+".opus", "user")
	}

	qd, _ := guildManager.GetQueue(guildID)
	qd.mu.Lock()
	originalOrder := make([]string, len(qd.Songs))
	for i, song := range qd.Songs {
		originalOrder[i] = song.Filename
	}
	qd.mu.Unlock()

	err := ShuffleGuildQueue(guildID)
	assert.NoError(t, err)

	qd.mu.Lock()
	defer qd.mu.Unlock()

	assert.Equal(t, len(originalOrder), len(qd.Songs))

	// Check if all original songs are still present (same set, different order)
	foundCount := 0
	for _, original := range originalOrder {
		for _, current := range qd.Songs {
			if current.Filename == original {
				foundCount++
				break
			}
		}
	}

	assert.Equal(t, len(originalOrder), foundCount)
}

func TestShuffleGuildQueue_NonExistent(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	err := ShuffleGuildQueue("non-existent-guild")
	assert.Error(t, err)
}

func TestEnqueue_StoppedSession(t *testing.T) {
	guildManager = &GuildManager{
		songs:    make(map[string]*QueueData),
		sessions: make(map[string]*SessionData),
	}

	guildID := "test-guild-stopped"

	// Create initial session and mark it as stopped
	Enqueue(guildID, "cache/song1.opus", "user1")
	sd, _ := guildManager.GetSession(guildID)
	sd.mu.Lock()
	sd.Session.stopped = true
	sd.mu.Unlock()

	// Enqueue another song
	gq := Enqueue(guildID, "cache/song2.opus", "user2")

	// Should create a new session since the old one was stopped
	sd.mu.Lock()
	defer sd.mu.Unlock()

	assert.False(t, sd.Session.stopped)
	assert.Equal(t, 2, len(gq.Songs))
}
