// Package config reads the app's settings from environment variables.
//
// Every setting has a sensible default so the app runs with no .env file at
// all. In development a .env file (see .env.example) is loaded first, if present.
package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds every setting the app needs. It is built once at startup and
// passed down explicitly - no package reads os.Getenv on its own.
type Config struct {
	Port   string // HTTP port to listen on, e.g. "8080"
	AppEnv string // "development" or "production"
}

// Load reads .env (if it exists) and then the environment.
func Load() Config {
	// A missing .env file is fine: in production the variables come from the
	// real environment. So the error is ignored on purpose.
	_ = godotenv.Load()

	return Config{
		Port:   getEnv("PORT", "8080"),
		AppEnv: getEnv("APP_ENV", "development"),
	}
}

// IsProduction reports whether the app runs in production mode.
func (c Config) IsProduction() bool {
	return c.AppEnv == "production"
}

// getEnv returns the variable's value, or fallback when it is unset or empty.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
