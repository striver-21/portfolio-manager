package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://u:p@localhost/db",
		"JWT_SECRET":   strings.Repeat("s", 32),
	}))
	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "portfolio-manager", cfg.JWTIssuer)
	assert.Equal(t, 24*time.Hour, cfg.TokenTTL)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"PORT":         "9090",
		"DATABASE_URL": "postgres://u:p@localhost/db",
		"JWT_SECRET":   strings.Repeat("s", 40),
		"JWT_ISSUER":   "acme",
		"TOKEN_TTL":    "15m",
		"LOG_LEVEL":    "debug",
	}))
	require.NoError(t, err)
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "acme", cfg.JWTIssuer)
	assert.Equal(t, 15*time.Minute, cfg.TokenTTL)
	assert.Equal(t, slog.LevelDebug, cfg.LogLevel)
}

func TestLoadReportsEveryProblem(t *testing.T) {
	_, err := Load(env(map[string]string{
		"JWT_SECRET": "short",
		"TOKEN_TTL":  "soon",
		"LOG_LEVEL":  "loud",
	}))
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "DATABASE_URL is required")
	assert.Contains(t, msg, "JWT_SECRET must be at least 32 bytes")
	assert.Contains(t, msg, "TOKEN_TTL")
	assert.Contains(t, msg, "LOG_LEVEL")
}

func TestLoadRequiresSecret(t *testing.T) {
	_, err := Load(env(map[string]string{"DATABASE_URL": "x"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_SECRET is required")

	_, err = Load(env(map[string]string{"DATABASE_URL": "x", "JWT_SECRET": strings.Repeat("s", 32), "TOKEN_TTL": "-1h"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TOKEN_TTL must be positive")
}
