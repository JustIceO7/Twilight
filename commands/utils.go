package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// connectUserVoiceChannel connects the bot to the voice channel the specified user is currently in.
func connectUserVoiceChannel(s *discordgo.Session, guildID, userID string) (*discordgo.VoiceConnection, error) {
	vcState, err := s.State.VoiceState(guildID, userID)
	if err != nil || vcState == nil {
		return nil, fmt.Errorf("user not in a voice channel")
	}

	vc, err := s.ChannelVoiceJoin(guildID, vcState.ChannelID, false, false)
	if err != nil {
		return nil, err
	}

	return vc, nil
}
