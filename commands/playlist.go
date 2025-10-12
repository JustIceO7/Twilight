package commands

import (
	"Twilight/db_client"
	"Twilight/playlist"
	"Twilight/redis_client"
	"context"

	"github.com/bwmarrin/discordgo"
)

// playList handles all the handlers of playlist commands
func playList(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	data := i.ApplicationCommandData()
	options := data.Options

	value := ""

	subCmd := options[0].Name
	if len(options[0].Options) > 0 {
		value = options[0].Options[0].StringValue()
	}

	pm := playlist.NewManager(s, redis_client.RDB, db_client.DB)
	pm.EnsureUserExists(i)

	switch subCmd {
	case "view":
		pm.ShowPlaylist(i)
	case "add":
		pm.AddSong(i, value)
	case "addplaylist":
		pm.AddPlaylist(i, value)
	case "remove":
		pm.RemoveSong(i, value)
	case "clear":
		pm.ClearPlaylist(i)
	case "play":
		// Check if user is in a voice channel and bot is not in a different one
		if !checkUserVoiceChannel(s, i) {
			return nil
		}
		vc, err := connectUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
		if err != nil {
			return nil
		}
		pm.PlaySong(i, value, vc)
	default:
		pm.ShowPlaylist(i)
	}
	return nil
}
