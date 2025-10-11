package handlers

import (
	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

// HelpEmbedding creates the embedding for the music commands help menu
func HelpEmbedding(s *discordgo.Session, m *discordgo.MessageCreate) {
	botAvatarURL := s.State.User.AvatarURL("64")
	helpEmbed := &discordgo.MessageEmbed{
		Title:       "Twilight Help",
		Description: "Use these slash commands to control music playback.",
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: botAvatarURL,
		},
		Color: viper.GetInt("theme"),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "__Music Commands__",
				Value: "`/play <url>` - Play a song from a YouTube URL.\n" +
					"`/playplaylist <url>` - Play a playlist from a YouTube URL.\n" +
					"`/pause` - Pause the current song.\n" +
					"`/resume` - Resume the paused song.\n" +
					"`/skip` - Skip the current song.\n" +
					"`/shuffle` - Shuffle the current song queue.\n" +
					"`/queue` - Show the current song queue.\n" +
					"`/np` - Show the song that's now playing.\n" +
					"`/sinfo` - Show the song info from a YouTube URL.\n" +
					"`/loop` - Toggle loop for the current song queue.\n" +
					"`/clear` - Clear the song queue and stop the current song.\n" +
					"`/disconnect` - Stop playback and disconnect the bot from the voice channel.\n" +
					"`/leave` - Stop playback and disconnect the bot from the voice channel.",
				Inline: false,
			},
			{
				Name: "__Playlist Commands__",
				Value: "`/playlist view` - View your playlist.\n" +
					"`/playlist add <song>` - Add a song to your playlist (YouTube video ID).\n" +
					"`/playlist remove <song>` - Remove a song from your playlist (YouTube video ID).\n" +
					"`/playlist clear` - Clear your playlist.\n" +
					"`/playlist play [song]` - Play a song from your playlist or the entire playlist (optional YouTube video ID).",
				Inline: false,
			},
		},
	}
	s.ChannelMessageSendEmbed(m.ChannelID, helpEmbed)
}
