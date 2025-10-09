package playlist

import (
	"Twilight/db_client"
	"Twilight/queue"
	"Twilight/redis_client"
	"Twilight/utils"
	"Twilight/yt"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

type PlaylistManager struct {
	session *discordgo.Session
	redis   *redis.Client
	db      *gorm.DB
}

type User struct {
	UserID   int64      `gorm:"primaryKey"`
	Playlist []Playlist `gorm:"foreignKey:UserID"`
}

type Song struct {
	ID          string `gorm:"primaryKey"`
	Title       string
	Author      string
	Views       int
	Description string
	Duration    int64 // Seconds
	PublishDate time.Time
	URL         string
}

type Playlist struct {
	UserID int64
	SongID string
	Song   Song `gorm:"foreignKey:SongID"`
}

const SONGS_PER_PAGE = 10

// ShowPlaylist displays a users playlist
func (pm *PlaylistManager) ShowPlaylist(i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)

	var playlist []Playlist
	if err := pm.db.Where("user_id = ?", userID).Preload("Song").Find(&playlist).Error; err != nil || len(playlist) == 0 {
		content := "Looks like your playlist is empty. Add some songs to get started! 🎵"
		if err != nil {
			content = "Oops! Something went wrong while fetching your playlist. 😅"
		}
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: content,
		})
		return
	}

	embed := CreatePlaylistEmbed(playlist, 0, SONGS_PER_PAGE)
	msg, _ := pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	})

	if len(playlist) > SONGS_PER_PAGE {
		pm.session.MessageReactionAdd(i.ChannelID, msg.ID, "◀️")
		pm.session.MessageReactionAdd(i.ChannelID, msg.ID, "▶️")
	}
}

// CreatePlaylistEmbed creates the embedding to be shown for displaying playlist
func CreatePlaylistEmbed(playlist []Playlist, page int, perPage int) *discordgo.MessageEmbed {
	totalPages := (len(playlist) + perPage - 1) / perPage
	if page < 0 {
		page = 0
	} else if page >= totalPages {
		page = totalPages - 1
	}

	start := page * perPage
	end := start + perPage
	if end > len(playlist) {
		end = len(playlist)
	}

	text := ""
	for i, p := range playlist[start:end] {
		text += fmt.Sprintf("%d. `%s`\n\u00A0\u00A0🔗 Video ID: `%s`\n", start+i+1, p.Song.Title, p.Song.ID)
	}

	return &discordgo.MessageEmbed{
		Title: "Your Playlist",
		Fields: []*discordgo.MessageEmbedField{
			{Name: "\u200B", Value: text},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Page %d/%d", page+1, totalPages),
		},
		Color: viper.GetInt("theme"),
	}
}

// HandlePlaylistReactions is responisble for controlling reaction reponses within playlistEmbed
func HandlePlaylistReactions(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if r.UserID == s.State.User.ID || (r.Emoji.Name != "◀️" && r.Emoji.Name != "▶️") {
		return
	}
	msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil || len(msg.Embeds) == 0 {
		return
	}
	embed := msg.Embeds[0]
	if embed.Title != "Your Playlist" || msg.Author.ID != s.State.User.ID {
		return
	}
	footer := embed.Footer
	if footer == nil {
		return
	}
	var currentPage int
	fmt.Sscanf(footer.Text, "Page %d/", &currentPage)
	currentPage--
	userID, _ := strconv.ParseInt(r.UserID, 10, 64)
	var playlist []Playlist
	if err := db_client.DB.Where("user_id = ?", userID).Preload("Song").Find(&playlist).Error; err != nil {
		return
	}
	totalPages := (len(playlist) + SONGS_PER_PAGE - 1) / SONGS_PER_PAGE
	newPage := currentPage
	if r.Emoji.Name == "▶️" {
		newPage++
		if newPage >= totalPages {
			newPage = 0
		}
	} else if r.Emoji.Name == "◀️" {
		newPage--
		if newPage < 0 {
			newPage = totalPages - 1
		}
	}
	embed = CreatePlaylistEmbed(playlist, newPage, SONGS_PER_PAGE)
	s.ChannelMessageEditEmbed(r.ChannelID, r.MessageID, embed)
	s.MessageReactionRemove(r.ChannelID, r.MessageID, r.Emoji.Name, r.UserID)
}

// AddSong adds a given song url to users playlist
func (pm *PlaylistManager) AddSong(i *discordgo.InteractionCreate, url string) {
	userID, err := strconv.ParseInt(i.Member.User.ID, 10, 64)
	if err != nil {
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: "Invalid user ID",
		})
		return
	}
	ytManager := yt.NewYouTubeManager(redis_client.RDB)

	data, err := ytManager.GetVideoMetadata(url)
	if err != nil {
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "Oops! Something went wrong while adding to your playlist. 😅",
		})
		return
	}

	song := Song{
		ID:          data.ID,
		Title:       data.Title,
		Author:      data.Author,
		Views:       data.Views,
		Description: data.Description,
		Duration:    int64(data.Duration.Seconds()),
		PublishDate: data.PublishDate,
		URL:         url,
	}

	if err := pm.db.FirstOrCreate(&song, Song{ID: data.ID}).Error; err != nil {
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: "Failed to save `" + song.Title + "` to database",
		})
		return
	}

	var existing Playlist
	if err := pm.db.Where("user_id = ? AND song_id = ?", userID, song.ID).First(&existing).Error; err == nil {
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: "`" + song.Title + "` is already in your playlist",
		})
		return
	}

	if err := pm.db.Create(&Playlist{UserID: userID, SongID: song.ID}).Error; err != nil {
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: "Failed to add `" + song.Title + "` to playlist",
		})
		return
	}

	pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Flags:   1 << 6, // Whisper Flag
		Content: "`" + song.Title + "` added to playlist",
	})
}

// RemoveSong removes song from users playlist given YouTube videoID
func (pm *PlaylistManager) RemoveSong(i *discordgo.InteractionCreate, songID string) {
	userID, err := strconv.ParseInt(i.Member.User.ID, 10, 64)
	if err != nil {
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: "Invalid user ID",
		})
		return
	}

	if err := pm.removeSong(userID, songID); err != nil {
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: "Failed to remove song `" + songID + "`",
		})
		return
	}

	pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Flags:   1 << 6, // Whisper Flag
		Content: "Song `" + songID + "` removed",
	})
}

// ClearPlaylist is responsible for cleaning the users playlist
func (pm *PlaylistManager) ClearPlaylist(i *discordgo.InteractionCreate) {
	userID, err := strconv.ParseInt(i.Member.User.ID, 10, 64)
	if err != nil {
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: "Invalid user ID",
		})
		return
	}

	var playlist []Playlist
	pm.db.Where("user_id = ?", userID).Find(&playlist)

	for _, p := range playlist {
		pm.removeSong(userID, p.SongID)
	}

	pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Flags:   1 << 6, // Whisper Flag
		Content: "All done! Your playlist has been cleared. ✨",
	})
}

// PlaySong plays a song given videoID from the users playlist, if omitted plays the entire users playlist
func (pm *PlaylistManager) PlaySong(i *discordgo.InteractionCreate, songID string, voiceConnection *discordgo.VoiceConnection) {
	userID, err := strconv.ParseInt(i.Member.User.ID, 10, 64)
	if err != nil {
		pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: "Invalid user ID",
		})
		return
	}

	var videoIDs []string
	var initialMsg *discordgo.Message
	songID, err = youtube.ExtractVideoID(songID) // Works with URLs as well
	if songID == "" {
		// Playing entire playlist
		var playlist []Playlist
		if err := pm.db.Where("user_id = ?", userID).Preload("Song").Find(&playlist).Error; err != nil {
			pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Flags:   1 << 6, // Whisper Flag
				Content: "Oops! Something went wrong while fetching your playlist. 😅",
			})
			return
		}

		if len(playlist) == 0 {
			pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Flags:   1 << 6, // Whisper Flag
				Content: "Looks like your playlist is empty. Add some songs to get started! 🎵",
			})
			return
		}

		for _, p := range playlist {
			videoIDs = append(videoIDs, p.Song.ID)
		}

		initialMsg, err = pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: fmt.Sprintf("Queuing %d song(s) from your playlist...", len(videoIDs)),
		})
		if err != nil {
			fmt.Printf("Failed to create initial message: %v\n", err)
			return
		}
	} else {
		// Playing selected song
		var playlist Playlist
		if err := pm.db.Where("user_id = ? AND song_id = ?", userID, songID).Preload("Song").First(&playlist).Error; err != nil {
			pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Flags:   1 << 6, // Whisper Flag
				Content: "Song not found in your playlist",
			})
			return
		}

		videoIDs = []string{playlist.Song.ID}

		initialMsg, err = pm.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Flags:   1 << 6, // Whisper Flag
			Content: fmt.Sprintf("Queuing `%s`...", playlist.Song.Title),
		})
		if err != nil {
			fmt.Printf("Failed to create initial message: %v\n", err)
			return
		}
	}

	go pm.processPlaylistSongs(videoIDs, i, voiceConnection, initialMsg)
}

// processPlaylistSongs handles downloading and queuing songs
func (pm *PlaylistManager) processPlaylistSongs(videoIDs []string, i *discordgo.InteractionCreate, vc *discordgo.VoiceConnection, initialMsg *discordgo.Message) {
	successCount := 0
	var filenames []string
	ytManager := yt.NewYouTubeManager(redis_client.RDB)

	for _, videoID := range videoIDs {
		filename := utils.GetAudioFile(videoID)

		if _, err := os.Stat(filename); os.IsNotExist(err) {
			err := ytManager.DownloadAudio(videoID)
			if err != nil {
				fmt.Printf("DEBUG: Download error: %v\n", err)
				continue
			}
		}

		filenames = append(filenames, filename)
		successCount++
	}

	if successCount == 0 {
		failMsg := "Oops! Couldn't download any songs from your playlist. 😅"
		pm.session.FollowupMessageEdit(i.Interaction, initialMsg.ID, &discordgo.WebhookEdit{
			Content: &failMsg,
		})
		return
	}

	gq, exists := queue.GetGuildQueue(i.GuildID)
	shouldStartPlayback := !exists || gq.Session.VC == nil || gq.CurrentSong == nil

	for _, filename := range filenames {
		queue.Enqueue(i.GuildID, filename, i.Member.User.Username)
	}

	if shouldStartPlayback {
		go queue.PlayNext(pm.session, i.GuildID, vc)
	}

	// Update user with final status by editing the initial message
	var finalContent string
	if successCount < len(videoIDs) {
		failedCount := len(videoIDs) - successCount
		if failedCount == 1 {
			finalContent = fmt.Sprintf("✅ Added %d songs to queue! (1 song couldn't be played)", successCount)
		} else {
			finalContent = fmt.Sprintf("✅ Added %d songs to queue! (%d songs couldn't be played)", successCount, failedCount)
		}
	} else {
		if successCount == 1 {
			finalContent = "🎵 Song added to queue!"
		} else {
			finalContent = fmt.Sprintf("🎵 All `%d` songs added to queue!", successCount)
		}
	}

	pm.session.FollowupMessageEdit(i.Interaction, initialMsg.ID, &discordgo.WebhookEdit{
		Content: &finalContent,
	})
}

// Newmanager returns a new instance of PlayListManager
func NewManager(s *discordgo.Session, r *redis.Client, db *gorm.DB) *PlaylistManager {
	return &PlaylistManager{
		session: s,
		redis:   r,
		db:      db,
	}
}

// removeSong removes a song from a users playlist, cleaning up any unused song entries
func (pm *PlaylistManager) removeSong(userID int64, songID string) error {
	if err := pm.db.Where("user_id = ? AND song_id = ?", userID, songID).Delete(&Playlist{}).Error; err != nil {
		return err
	}

	if err := pm.db.Exec(`
		DELETE FROM songs
		WHERE id = ?
		  AND NOT EXISTS (
			SELECT 1 FROM playlists WHERE song_id = ?
		  )
	`, songID, songID).Error; err != nil {
		return err
	}

	return nil
}

// EnsureUserExists checks if user exists within the database, else it creates an entry for the user
func (pm *PlaylistManager) EnsureUserExists(i *discordgo.InteractionCreate) error {
	userID, err := strconv.ParseInt(i.Member.User.ID, 10, 64)
	if err != nil {
		return err
	}

	user := User{UserID: userID}
	return pm.db.FirstOrCreate(&user, User{
		UserID: userID,
	}).Error
}
