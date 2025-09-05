package main

import (
	"Twilight/commands"
	"Twilight/config"
	"Twilight/handlers"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/Strum355/log"
	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

var production *bool

func main() {
	// Sets Flag to Debug Mode
	production = flag.Bool("p", false, "enables production with json logging")
	flag.Parse()
	if *production {
		log.InitJSONLogger(&log.Config{Output: os.Stdout})
	} else {
		log.InitSimpleLogger(&log.Config{Output: os.Stdout})
	}

	// Sets up Configurations for Viper
	config.InitConfig()

	// Creates Discord Bot Session
	s, err := discordgo.New("Bot " + viper.GetString("discord.token"))
	if err != nil {
		log.WithError(err)
		return
	}

	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Info("Bot has registered handlers")
	})

	// Configuring Intents and Adding Handlers
	handlers.HandlerConfig(s)

	// Register Slash and Component Commands
	commands.RegisterSlashCommands(s)

	// Connecting to Discord Server Gateway
	s.Open()
	log.Info("Bot is initialising")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
	log.Info("Cleanly exiting")
	s.Close()
}
