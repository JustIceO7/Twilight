package config

import (
	"strings"

	"github.com/Strum355/log"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

func InitConfig() {
	if err := godotenv.Load(); err != nil {
		log.Info("No .env file found, proceeding with defaults.")
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	initDefaults()
	viper.AutomaticEnv()
}
