// Package config reads the app's settings from environment variables.
//
// Every setting has a sensible default so the app runs with no .env file at
// all. In development a .env file (see .env.example) is loaded first, if present.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds every setting the app needs. It is built once at startup and
// passed down explicitly - no package reads os.Getenv on its own.
type Config struct {
	Port   string // HTTP port to listen on, e.g. "8080"
	AppEnv string // "development" or "production"

	// AI model server. Any OpenAI-compatible server works:
	//   Ollama:    http://localhost:11434
	//   llama.cpp: http://localhost:8080 (llama-server), so change PORT then
	LLMBaseURL         string
	LLMModel           string
	LLMAPIKey          string        // empty for local Ollama
	LLMReasoningEffort string        // "none" = no thinking (faster); "" = model default
	LLMTimeout         time.Duration // max time for one answer

	// Logging
	LogLevel string // debug | info | warn | error

	// Rate limits per client IP. 0 turns a limit off.
	RateLimitCoachPerHour  int // questions to the AI Coach per hour
	RateLimitGeneralPerMin int // all other requests per minute
}

// Load reads .env (if it exists) and then the environment.
func Load() Config {
	// A missing .env file is fine: in production the variables come from the
	// real environment. So the error is ignored on purpose.
	_ = godotenv.Load()

	return Config{
		Port:   getEnv("PORT", "8080"),
		AppEnv: getEnv("APP_ENV", "development"),

		LLMBaseURL:         getEnv("LLM_BASE_URL", "http://localhost:11434"),
		LLMModel:           getEnv("LLM_MODEL", "qwen3.5:0.8b"),
		LLMAPIKey:          getEnv("LLM_API_KEY", ""),
		LLMReasoningEffort: getEnvAllowEmpty("LLM_REASONING_EFFORT", "none"),
		LLMTimeout:         getDuration("LLM_TIMEOUT", 3*time.Minute),

		LogLevel: getEnv("LOG_LEVEL", "info"),

		RateLimitCoachPerHour:  getInt("RATE_LIMIT_COACH_PER_HOUR", 30), // same as NutriVa
		RateLimitGeneralPerMin: getInt("RATE_LIMIT_GENERAL_PER_MIN", 120),
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

// getEnvAllowEmpty is like getEnv, but a variable that is set to "" stays "".
// Only a variable that is missing completely gets the fallback.
func getEnvAllowEmpty(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// getDuration parses values like "90s", "3m" or "1m30s".
// An invalid value logs a warning and uses the fallback.
func getDuration(key string, fallback time.Duration) time.Duration {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		slog.Warn("config: invalid duration, using default", "key", key, "value", v, "default", fallback)
		return fallback
	}
	return d
}

// getInt parses a whole number >= 0. An invalid value logs a warning and uses
// the fallback.
func getInt(key string, fallback int) int {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		slog.Warn("config: invalid number, using default", "key", key, "value", v, "default", fallback)
		return fallback
	}
	return n
}
