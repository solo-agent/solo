package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the application.
type Config struct {
	Port       string
	DBURL      string
	JWTSecret  string
	LogLevel   string

	// Daemon-specific configuration (used by cmd/daemon)
	ServerURL   string // Server URL for daemon registration (e.g., "http://localhost:8080")
	DaemonID    string // Unique ID for this daemon instance
	LLMAPIKey   string // API key for LLM provider
	LLMProvider string // LLM provider type ("openai" | "anthropic")

	// Attachment configuration
	AttachmentsDir string // Directory for uploaded file storage (default: ~/.solo/attachments)
}

// LoadDotenv reads .env from the working directory and sets environment variables
// for keys that are not already present in the environment. Existing env vars
// always take precedence (the developer can override .env values).
func LoadDotenv() error {
	f, err := os.Open(".env")
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip inline comments (handle VAL=foo # comment)
		if idx := strings.Index(line, " #"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		eq := strings.Index(line, "=")
		if eq < 1 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Remove surrounding quotes
		if len(val) >= 2 && val[0] == val[len(val)-1] && (val[0] == '"' || val[0] == '\'') {
			val = val[1 : len(val)-1]
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return sc.Err()
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:      getEnv("PORT", "8080"),
		DBURL:     getEnv("DATABASE_URL", "postgres://solo:solo@localhost:5432/solo?sslmode=disable"),
		JWTSecret: getEnv("JWT_SECRET", "solo-dev-secret-change-in-production"),
		LogLevel:  getEnv("LOG_LEVEL", "debug"),

		// Daemon config
		ServerURL:   getEnv("DAEMON_SERVER_URL", "http://localhost:8080"),
		DaemonID:    getEnv("DAEMON_ID", "daemon-01"),
		LLMAPIKey:   getEnv("LLM_API_KEY", ""),
		LLMProvider: getEnv("LLM_PROVIDER", "anthropic"),

		// Attachment config
		AttachmentsDir: getEnv("ATTACHMENTS_DIR", expandHome("~/.solo/attachments")),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// expandHome replaces a leading "~" with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return strings.Replace(path, "~", home, 1)
		}
	}
	return path
}

// GetEnvDuration reads a duration from an env var.
func GetEnvDuration(name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

// GetEnvInt reads an int from an env var.
func GetEnvInt(name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}
