package utils

import (
	"os"

	"github.com/joho/godotenv"
)

func init() {
	// Make sure load ENV variables from .env and .env.local files once
	godotenv.Load(".env", ".env.local")
}

// Returns the ENV vars including those from the .env and .env.local files.
// When the variable is empty/not set, it returns the default value.
func GetEnvWithDefault(key, defaultValue string) string {
	if val := GetEnv(key); val != "" {
		return val
	}
	return defaultValue
}

// Returns the ENV vars including those from the .env and .env.local files.
func GetEnv(key string) string {
	return os.Getenv(key)
}

func SetEnv(key, value string) {
	os.Setenv(key, value)
}

func UnsetEnv(key string) {
	os.Unsetenv(key)
}
