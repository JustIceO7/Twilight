package commands

import (
	"context"
	"errors"

	"github.com/Strum355/log"
	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

// RegisterSlashCommands adds all slash commands to the session.
func RegisterSlashCommands(s *discordgo.Session) {
	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "play",
			Description: "Play a song from a Youtube URL.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "url",
					Description: "Youtube link for the song",
					Required:    true,
				},
			},
		},
		playSong,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "pause",
			Description: "Pause the current song.",
		},
		pauseSong,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "resume",
			Description: "Resume the paused song.",
		},
		resumeSong,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "loop",
			Description: "Toggles loop for the current queue.",
		},
		loopQueue,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "sinfo",
			Description: "Shows info of song.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "url",
					Description: "Youtube link for the song",
					Required:    true,
				},
			},
		},
		songInfo,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "skip",
			Description: "Skip the current song.",
		},
		skipSong,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "queue",
			Description: "Show the current song queue.",
		},
		currentQueue,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "shuffle",
			Description: "Shuffle the current song queue.",
		},
		shuffleQueue,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "np",
			Description: "Show the song that’s now playing.",
		},
		currentSong,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "playlist",
			Description: "Manage your personal playlist",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "view",
					Description: "View your current playlist",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "add",
					Description: "Add a song to your playlist",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "song",
							Description: "YouTube video ID",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "remove",
					Description: "Remove a song from your playlist",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "song",
							Description: "YouTube video ID",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "clear",
					Description: "Clear your entire playlist",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "play",
					Description: "Start a song within your playlist or play the entire playlist",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "song",
							Description: "YouTube video ID",
							Required:    false,
						},
					},
				},
			},
		},
		playList,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "disconnect",
			Description: "Disconnect the bot from voice chat.",
		},
		stopSong,
	)

	commands.Add(
		&discordgo.ApplicationCommand{
			Name:        "leave",
			Description: "Disconnect the bot from voice chat.",
		},
		stopSong,
	)

	if err := commands.Register(s); err != nil {
		log.WithError(err).Error("Failed to register slash commands")
	}
}

type CommandHandler func(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) *interactionError

type Commands struct {
	commands          []*discordgo.ApplicationCommand
	handlers          map[string]CommandHandler
	componentHandlers map[string]CommandHandler
}

var (
	commands = &Commands{}
)

// Adds command to the slash commands.
func (c *Commands) Add(com *discordgo.ApplicationCommand, handler CommandHandler) {
	c.commands = append(c.commands, com)
	if c.handlers == nil {
		c.handlers = map[string]CommandHandler{}
	}
	c.handlers[com.Name] = handler
}

// Adds command to component commands
func (c *Commands) AddComponent(name string, handler CommandHandler) {
	if c.componentHandlers == nil {
		c.componentHandlers = map[string]CommandHandler{}
	}
	c.componentHandlers[string(name[0])] = handler
}

// Register all slash commands and component commands
func (c *Commands) Register(s *discordgo.Session) error {
	// Handles all interactions and routes them to the correct command handler
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			callCommandHandler(s, i)
		case discordgo.InteractionMessageComponent:
			callComponentHandler(s, i)
		}
	})

	// Registers slash commands
	if _, err := s.ApplicationCommandBulkOverwrite(viper.GetString("discord.app.id"), "", c.commands); err != nil {
		log.WithError(err).Error("Failed to create commands")
		return err
	}
	return nil
}

// Cannot be an interaction through DMs
func checkDirectMessage(i *discordgo.InteractionCreate) (*discordgo.User, *interactionError) {
	if i.GuildID == "" {
		return nil, &interactionError{
			errors.New("command invoked outside of valid guild"),
			"This command is only available in a valid server",
		}
	}
	return i.Member.User, nil
}

// Component or button based interactions
func callComponentHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
	m := i.MessageComponentData()
	if m.CustomID == "" {
		iErr := &interactionError{
			errors.New("No custom_id assigned to component on message " + i.Message.ID),
			"Couldn't handle component, invalid custom_id",
		}
		iErr.Handle(s, i)
		return
	}
	commandLabel := string(m.CustomID[0])
	if handler, ok := commands.componentHandlers[commandLabel]; ok {
		ctx := context.WithValue(ctx, log.Key, log.Fields{
			"user_id":          i.Member.User.ID,
			"channel_id":       i.ChannelID,
			"guild_id":         i.GuildID,
			"user":             i.Member.User.Username,
			"interaction_type": "component",
			"command":          commandLabel,
		})
		log.WithContext(ctx).Info("Invoking component command")
		iErr := handler(ctx, s, i)
		if iErr != nil {
			iErr.Handle(s, i)
		}
	}
}

// Text or slash command interactions
func callCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var iError *interactionError
	ctx := context.Background()
	commandAuthor, iError := checkDirectMessage(i)
	if iError != nil {
		iError.Handle(s, i)
		return
	}

	commandName := i.ApplicationCommandData().Name

	channel, err := s.Channel(i.ChannelID)
	if err != nil {
		iError = &interactionError{err, "Couldn't query channel"}
		iError.Handle(s, i)
		return
	}

	if handler, ok := commands.handlers[commandName]; ok {
		ctx := context.WithValue(ctx, log.Key, log.Fields{
			"author_id":        commandAuthor.ID,
			"channel_id":       i.ChannelID,
			"guild_id":         i.GuildID,
			"user":             commandAuthor.Username,
			"channel_name":     channel.Name,
			"interaction_type": "application",
			"command":          commandName,
		})
		log.WithContext(ctx).Info("Invoking application command")
		iError = handler(ctx, s, i)
		if iError != nil {
			iError.Handle(s, i)
		}
	}
}
