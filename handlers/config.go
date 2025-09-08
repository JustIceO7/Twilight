package handlers

import (
	"Twilight/playlist"

	"github.com/bwmarrin/discordgo"
)

// HandlerConfig handles configs for intents and handlers
func HandlerConfig(s *discordgo.Session) {
	s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuildMessageReactions | discordgo.IntentsGuilds | discordgo.IntentsGuildVoiceStates | discordgo.IntentsMessageContent
	s.AddHandler(MessageHandler)
	s.AddHandler(playlist.HandlePlaylistReactions)
}
