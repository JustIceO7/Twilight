package commands

import (
	"github.com/bwmarrin/discordgo"
)

// connectUserVoiceChannel connects the bot to the voice channel the specified user is currently in.
func connectUserVoiceChannel(s *discordgo.Session, guildID, userID string) (*discordgo.VoiceConnection, error) {
	vcState, err := s.State.VoiceState(guildID, userID)
	if err != nil || vcState == nil {
		return nil, err
	}

	if vc, ok := s.VoiceConnections[guildID]; ok && vc != nil {
		if vc.ChannelID == vcState.ChannelID {
			return vc, nil
		}
		return nil, err
	}

	vc, err := s.ChannelVoiceJoin(guildID, vcState.ChannelID, false, false)
	if err != nil {
		return nil, err
	}

	return vc, nil
}

// checkUserVoiceChannel checks whether user is in the same voice channel as bot
func checkUserVoiceChannel(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	// Get user's current voice channel
	vs, err := s.State.VoiceState(i.GuildID, i.Member.User.ID)
	if err != nil || vs == nil || vs.ChannelID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Join a voice channel first 😉",
			},
		})
		return false
	}

	// Check if bot is already in a different voice channel
	if vc, ok := s.VoiceConnections[i.GuildID]; ok && vc != nil && vc.ChannelID != vs.ChannelID {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "I'm already in another voice channel 😅",
			},
		})
		return false
	}

	return true
}
