package commands

import (
	"github.com/Strum355/log"
	"github.com/bwmarrin/discordgo"
)

type interactionError struct {
	err     error
	message string
}

// Handle handles responding to error messages within Discord
func (e *interactionError) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.WithError(e.err).Error(e.message)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   1 << 6, // Whisper Flag
			Content: e.message,
		},
	})
}
