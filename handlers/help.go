package handlers

import (
	"github.com/bwmarrin/discordgo"
)

// HelpEmbedding creates the embedding for the help menu
func HelpEmbedding(s *discordgo.Session, m *discordgo.MessageCreate) {
	botAvatarURL := s.State.User.AvatarURL("64")
	helpEmbed := &discordgo.MessageEmbed{
		Title: "Twilight Help",
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: botAvatarURL,
		},
	}
	s.ChannelMessageSendEmbed(m.ChannelID, helpEmbed)
}
