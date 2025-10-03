package main

import (
	"Twilight/commands"
	"Twilight/config"
	"Twilight/db_client"
	"Twilight/handlers"
	"Twilight/queue"
	"Twilight/redis_client"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Strum355/log"
	"github.com/bwmarrin/discordgo"
	"github.com/redis/go-redis/v9"
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

	db_client.Init()

	StartCacheCleaning()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
	gracefulShutdown(s)
}

// gacefulShutdown handles cleaning up after the bot is shutdown
func gracefulShutdown(s *discordgo.Session) {
	log.Info("Starting graceful shutdown...")

	queue.StopAllSessions()

	for _, vc := range s.VoiceConnections {
		if vc != nil {
			vc.Disconnect()
		}
	}

	time.Sleep(5 * time.Second)

	s.Close()

	cleanUpCache()

	log.Info("Cleanly exiting")
}

// cleanUpCache removes old cached audio files
func cleanUpCache() {
	cacheDir := "cache"
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return
	}

	files, _ := os.ReadDir(cacheDir)

	for _, file := range files {
		_ = os.RemoveAll(cacheDir + "/" + file.Name())
	}

	log.Info("Cache cleanup completed")
}

// StartCacheCleaning starts the mp3 background cleanup
func StartCacheCleaning() {
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			routineCacheCleaning()
		}
	}()
}

// routineCacheCleaning cleans up mp3 files which have been unused
func routineCacheCleaning() {
	log.Info("Beginning cache cleanup!")
	cacheDir := "cache"
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return
	}

	files, _ := os.ReadDir(cacheDir)

	for _, file := range files {
		_, err := redis_client.RDB.Get(redis_client.Ctx, "video:"+strings.TrimSuffix(file.Name(), ".mp3")).Result()
		if err == redis.Nil {
			_ = os.Remove(cacheDir + "/" + file.Name())
		}
	}
}
