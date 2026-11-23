package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	WebSocket  WebSocketConfig
	Auth       AuthConfig
	Log        LogConfig
	Migrations MigrationsConfig
}

// LogConfig holds structured logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// MigrationsConfig holds database migration options
type MigrationsConfig struct {
	AutoMigrate bool
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port string
	Host string
}

// AuthConfig holds JWT and authentication configuration
type AuthConfig struct {
	JWTSecret        string
	TokenExpiryHours int
}

// DatabaseConfig holds database-related configuration
type DatabaseConfig struct {
	User     string
	Password string
	Name     string
	Host     string
	Port     string
	SSLMode  string
}

// WebSocketConfig holds WebSocket-related configuration
type WebSocketConfig struct {
	ReadBufferSize  int
	WriteBufferSize int
	AllowedOrigins  []string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	config := &Config{
		Server: ServerConfig{
			Port: getEnv("PORT", "5000"),
			Host: getEnv("HOST", "localhost"),
		},
		Database: DatabaseConfig{
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Name:     getEnv("DB_NAME", "peeriodic"),
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		WebSocket: WebSocketConfig{
			ReadBufferSize:  getEnvAsInt("WS_READ_BUFFER_SIZE", 1024),
			WriteBufferSize: getEnvAsInt("WS_WRITE_BUFFER_SIZE", 1024),
			AllowedOrigins:  strings.Split(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), ","),
		},
		Auth: AuthConfig{
			JWTSecret:        os.Getenv("JWT_SECRET"),
			TokenExpiryHours: getEnvAsInt("JWT_EXPIRY_HOURS", 72),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "INFO"),
			Format: getEnv("LOG_FORMAT", "text"),
		},
		Migrations: MigrationsConfig{
			AutoMigrate: getEnvAsBool("AUTO_MIGRATE", true),
		},
	}

	// Validate required fields
	if config.Database.User == "" {
		return nil, fmt.Errorf("DB_USER environment variable is required")
	}
	if len(config.Auth.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	for _, key := range []string{"PORT", "DB_PORT", "JWT_EXPIRY_HOURS", "WS_READ_BUFFER_SIZE", "WS_WRITE_BUFFER_SIZE"} {
		if value := os.Getenv(key); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 || n > 65535 {
				return nil, fmt.Errorf("%s must be an integer between 1 and 65535", key)
			}
		}
	}
	if value := os.Getenv("AUTO_MIGRATE"); value != "" {
		if _, err := strconv.ParseBool(value); err != nil {
			return nil, fmt.Errorf("AUTO_MIGRATE must be a boolean")
		}
	}
	for i, origin := range config.WebSocket.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("ALLOWED_ORIGINS must contain HTTP or HTTPS origins without paths")
		}
		config.WebSocket.AllowedOrigins[i] = origin
	}

	return config, nil
}

// GetDatabaseConnectionString returns the formatted database connection string
func (c *Config) GetDatabaseConnectionString() string {
	dsn := url.URL{Scheme: "postgres", Host: net.JoinHostPort(c.Database.Host, c.Database.Port), Path: "/" + c.Database.Name, User: url.UserPassword(c.Database.User, c.Database.Password)}
	params := url.Values{"sslmode": {c.Database.SSLMode}, "connect_timeout": {"5"}}
	dsn.RawQuery = params.Encode()
	return dsn.String()
}

// GetServerAddress returns the formatted server address
func (c *Config) GetServerAddress() string {
	return net.JoinHostPort(c.Server.Host, c.Server.Port)
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
