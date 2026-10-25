// Package config loads runtime settings from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/striver-21/portfolio-manager/internal/auth"
)

// Config is everything the server reads from the environment. Each field maps
// to exactly one variable; .env.example lists the same set.
type Config struct {
	Port        string        // PORT, default 8080
	DatabaseURL string        // DATABASE_URL, required
	JWTSecret   []byte        // JWT_SECRET, required, >= 32 bytes
	JWTIssuer   string        // JWT_ISSUER, default "portfolio-manager"
	TokenTTL    time.Duration // TOKEN_TTL, default 24h
	LogLevel    slog.Level    // LOG_LEVEL, default info
}

// Load reads configuration via getenv (normally os.Getenv). It returns one
// error listing every missing or invalid variable so operators fix them in a
// single pass.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	cfg := Config{
		Port:        orDefault(getenv("PORT"), "8080"),
		DatabaseURL: strings.TrimSpace(getenv("DATABASE_URL")),
		JWTSecret:   []byte(getenv("JWT_SECRET")),
		JWTIssuer:   orDefault(getenv("JWT_ISSUER"), "portfolio-manager"),
		TokenTTL:    24 * time.Hour,
		LogLevel:    slog.LevelInfo,
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	switch n := len(cfg.JWTSecret); {
	case n == 0:
		errs = append(errs, errors.New("JWT_SECRET is required"))
	case n < auth.MinSecretLength:
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d bytes, got %d", auth.MinSecretLength, n))
	}
	if v := getenv("TOKEN_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("TOKEN_TTL: %w", err))
		case d <= 0:
			errs = append(errs, errors.New("TOKEN_TTL must be positive"))
		default:
			cfg.TokenTTL = d
		}
	}
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
		}
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
