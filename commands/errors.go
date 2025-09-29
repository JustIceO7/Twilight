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

// sendErrorResponse sends a generic error message to Discord
func sendErrorResponse(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "❌ An error occurred while processing your request."},
	})
}

func sendFetchErrorResponse(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "❌ Failed to fetch video details."},
	})
}
