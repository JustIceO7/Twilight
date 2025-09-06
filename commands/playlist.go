package commands

import (
	"Twilight/db_client"
	"Twilight/playlist"
	"Twilight/redis_client"
	"context"

	"github.com/bwmarrin/discordgo"
)

func playList(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError {
	data := i.ApplicationCommandData()
	options := data.Options

	action := "view"
	value := ""

	if len(options) > 0 {
		action = options[0].StringValue()
	}
	if len(options) > 1 {
		value = options[1].StringValue()
	}

	pm := playlist.NewManager(s, redis_client.RDB, db_client.DB, context.Background())

	switch action {
	case "view":
		pm.ShowPlaylist(i)
	case "add":
		pm.AddSong(i, value)
	case "remove":
		pm.RemoveSong(i, value)
	case "clear":
		pm.ClearPlaylist(i)
	case "play":
		pm.PlaySong(i, value)
	default:
		pm.ShowPlaylist(i)
	}
	return nil
}
