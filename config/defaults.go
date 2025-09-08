package config

import (
	"os"

	"github.com/spf13/viper"
)

func initDefaults() {
	viper.SetDefault("discord.token", os.Getenv("discord_token"))
	viper.SetDefault("discord.app.id", os.Getenv("discord_app_id"))
	viper.SetDefault("prefix", "^")
	viper.SetDefault("theme", 0xFF33CC) // Main theme of discord embeds
}
