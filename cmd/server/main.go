// Command server runs the portfolio-manager REST API.
//
//	server                 serve on $PORT (migrations must already be applied)
//	server -migrate        apply pending migrations, then serve
//	server -migrate-only   apply pending migrations and exit
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/striver-21/portfolio-manager/internal/auth"
	"github.com/striver-21/portfolio-manager/internal/config"
	"github.com/striver-21/portfolio-manager/internal/httpapi"
	"github.com/striver-21/portfolio-manager/internal/ratelimit"
	"github.com/striver-21/portfolio-manager/internal/store"
)

const (
	startupTimeout  = 15 * time.Second
	shutdownTimeout = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	migrate := flag.Bool("migrate", false, "apply pending database migrations before serving")
	migrateOnly := flag.Bool("migrate-only", false, "apply pending database migrations and exit")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	// Gin defaults to debug mode (verbose stdout logging, stack traces to
	// clients). Release unless the operator asked for something else.
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	db, err := store.OpenPostgres(startCtx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	if *migrate || *migrateOnly {
		applied, err := store.Migrate(ctx, db, store.PostgresMigrations())
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		logger.Info("migrations applied", "count", len(applied), "versions", applied)
		if *migrateOnly {
			return nil
		}
	}

	tokens, err := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.TokenTTL)
	if err != nil {
		return err
	}

	api := httpapi.New(httpapi.Options{
		Store:  store.NewGorm(db),
		Tokens: tokens,
		Logger: logger,
		// 10 auth attempts per minute per IP, with a burst of 10.
		AuthLimiter: ratelimit.New(10.0/60, 10),
	})

	srv := &http.Server{
		Addr:              net.JoinHostPort("", cfg.Port),
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down", "timeout", shutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("stopped")
	return nil
}
