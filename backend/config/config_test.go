package config

import (
	"net/url"
	"strings"
	"testing"
)

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"JWT_SECRET", ""}, {"JWT_SECRET", "short"}, {"PORT", "abc"}, {"DB_PORT", "70000"},
		{"JWT_EXPIRY_HOURS", "0"}, {"WS_READ_BUFFER_SIZE", "-1"}, {"AUTO_MIGRATE", "maybe"},
		{"ALLOWED_ORIGINS", "*"}, {"ALLOWED_ORIGINS", "https://example.com/path"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid configuration to fail")
			}
		})
	}
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionStringEscapesCredentials(t *testing.T) {
	cfg := &Config{Database: DatabaseConfig{User: "user@name", Password: "p@ss:/?#word", Host: "::1", Port: "5432", Name: "my database", SSLMode: "require"}, Server: ServerConfig{Host: "::1", Port: "5000"}}
	parsed, err := url.Parse(cfg.GetDatabaseConnectionString())
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if parsed.User.Username() != cfg.Database.User || password != cfg.Database.Password || parsed.Hostname() != "::1" || parsed.Path != "/my database" {
		t.Fatalf("DSN did not round-trip: %v", parsed.Redacted())
	}
	if cfg.GetServerAddress() != "[::1]:5000" {
		t.Fatal(cfg.GetServerAddress())
	}
}
