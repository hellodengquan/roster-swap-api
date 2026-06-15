package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port         string
	JWTSecret    string
	JWTExpireHours int
	DBPath        string
}

var AppConfig Config

func LoadConfig() error {
	_ = godotenv.Load()

	AppConfig = Config{
		Port:         getEnv("PORT", "8080"),
		JWTSecret:    getEnv("JWT_SECRET", "default-secret"),
		JWTExpireHours: getEnvAsInt("JWT_EXPIRE_HOURS", 24),
		DBPath:        getEnv("DB_PATH", "roster_swap.db"),
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
