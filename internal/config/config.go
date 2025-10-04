package config

import (
	"os"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
)

type Config struct {
	DatabaseURL   string
	Port          string
	GeminiAPIKey string
}

func Load() *Config {

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	if err := godotenv.Load(); err != nil {
		log.Warn(".env not found, using variable environments")
	}

	cfg := &Config{
		DatabaseURL:   getEnv("DATABASE_URL", ""),
		Port:          getEnv("PORT", "3001"),
		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	if cfg.GeminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY is not set")
	}

	log.Info("Config loaded")
	return cfg

}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}