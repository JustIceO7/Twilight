package config

import (
	"os"

	"github.com/spf13/viper"
)

func initDefaults() {
	viper.SetDefault("discord.token", os.Getenv("discord_token"))
	viper.SetDefault("discord.app.id", os.Getenv("discord_app_id"))
	viper.SetDefault("prefix", os.Getenv("prefix"))
	viper.SetDefault("theme", os.Getenv("theme")) // Main theme of discord embeds

	// Redis TTL timers in seconds
	viper.SetDefault("cache.audio", 7200)   // 2 hour
	viper.SetDefault("cache.youtube", 3600) // 1 hour

	viper.SetDefault("youtube.concurrency", 3) // Max concurrent downloads when downloading from YouTube concurrently
}
