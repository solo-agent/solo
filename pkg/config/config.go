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
	Port      string
	DBURL     string
	JWTSecret string
	LogLevel  string
	PublicURL string

	// Public account and email delivery configuration.
	AllowSignup           bool
	AllowedEmails         []string
	AllowedEmailDomains   []string
	AuthMailTransport     string
	SMTPHost              string
	SMTPPort              string
	SMTPUsername          string
	SMTPPassword          string
	SMTPFrom              string
	SMTPTLS               string
	TencentCloudSecretID  string
	TencentCloudSecretKey string
	TencentSESRegion      string
	TencentSESFrom        string
	TencentSESTemplateID  int

	// Daemon-specific configuration (used by cmd/daemon)
	ServerURL   string // Server URL for daemon registration (e.g., "http://127.0.0.1:8080")
	DaemonID    string // Unique ID for this daemon instance
	ComputerID  string // Server-side Computer binding for the outbound control connection
	ComputerKey string // Long-lived Computer credential; persisted locally after enrollment
	EnrollToken string // One-time Computer enrollment token
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
		DBURL:     getEnv("DATABASE_URL", "postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable"),
		JWTSecret: getEnv("JWT_SECRET", "solo-dev-secret-change-in-production"),
		LogLevel:  getEnv("LOG_LEVEL", "debug"),
		PublicURL: strings.TrimRight(getEnv("PUBLIC_URL", "http://localhost:3000"), "/"),

		AllowSignup:           getEnvBool("ALLOW_SIGNUP", true),
		AllowedEmails:         getEnvList("ALLOWED_EMAILS"),
		AllowedEmailDomains:   getEnvList("ALLOWED_EMAIL_DOMAINS"),
		AuthMailTransport:     strings.ToLower(strings.TrimSpace(getEnv("AUTH_MAIL_TRANSPORT", "smtp"))),
		SMTPHost:              strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:              getEnv("SMTP_PORT", "587"),
		SMTPUsername:          os.Getenv("SMTP_USERNAME"),
		SMTPPassword:          os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:              strings.TrimSpace(os.Getenv("SMTP_FROM")),
		SMTPTLS:               strings.ToLower(strings.TrimSpace(getEnv("SMTP_TLS", "starttls"))),
		TencentCloudSecretID:  strings.TrimSpace(os.Getenv("TENCENTCLOUD_SECRET_ID")),
		TencentCloudSecretKey: strings.TrimSpace(os.Getenv("TENCENTCLOUD_SECRET_KEY")),
		TencentSESRegion:      strings.TrimSpace(getEnv("TENCENT_SES_REGION", "ap-guangzhou")),
		TencentSESFrom:        strings.TrimSpace(os.Getenv("TENCENT_SES_FROM")),
		TencentSESTemplateID:  GetEnvInt("TENCENT_SES_TEMPLATE_ID", 0),

		// Daemon config
		ServerURL:   getEnv("DAEMON_SERVER_URL", "http://127.0.0.1:8080"),
		DaemonID:    getEnv("DAEMON_ID", "daemon-01"),
		ComputerID:  getEnv("SOLO_COMPUTER_ID", ""),
		ComputerKey: getEnv("SOLO_COMPUTER_CREDENTIAL", ""),
		EnrollToken: getEnv("SOLO_ENROLLMENT_TOKEN", ""),
		LLMAPIKey:   getEnv("LLM_API_KEY", ""),
		LLMProvider: getEnv("LLM_PROVIDER", "anthropic"),

		// Attachment config
		AttachmentsDir: getEnv("ATTACHMENTS_DIR", expandHome("~/.solo/attachments")),
	}
}

func getEnvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvList(key string) []string {
	parts := strings.Split(os.Getenv(key), ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.ToLower(strings.TrimSpace(part)); value != "" {
			values = append(values, value)
		}
	}
	return values
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
