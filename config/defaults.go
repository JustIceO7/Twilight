package config

import (
	"os"

	"github.com/spf13/viper"
)

func initDefaults() {
	viper.SetDefault("discord.token", os.Getenv("discord_token"))
	viper.SetDefault("discord.app.id", os.Getenv("discord_app_id"))
	viper.SetDefault("prefix", "^")
}
