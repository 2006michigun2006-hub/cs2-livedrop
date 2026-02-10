package config

import "os"

type Config struct {
	Port                string
	DatabaseURL         string
	JWTSecret           string
	BaseURL             string
	FrontendPath        string
	TelegramBotToken    string
	TelegramBotUsername string
}

func Load() Config {
	return Config{
		Port:                getEnv("PORT", "8080"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/livedrop?sslmode=disable"),
		JWTSecret:           getEnv("JWT_SECRET", "change-me-in-production"),
		BaseURL:             getEnv("BASE_URL", "http://localhost:8080"),
		FrontendPath:        getEnv("FRONTEND_PATH", "./web"),
		TelegramBotToken:    getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramBotUsername: getEnv("TELEGRAM_BOT_USERNAME", ""),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
