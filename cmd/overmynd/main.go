package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/z354prance/overmynd/internal/api"
	"github.com/z354prance/overmynd/internal/auth"

	"github.com/z354prance/overmynd/internal/database"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/lidarr"
	"github.com/z354prance/overmynd/internal/integrations/nzbget"
	"github.com/z354prance/overmynd/internal/integrations/qbittorrent"
	"github.com/z354prance/overmynd/internal/integrations/radarr"
	"github.com/z354prance/overmynd/internal/integrations/seerr"
	"github.com/z354prance/overmynd/internal/integrations/sonarr"
	"github.com/z354prance/overmynd/internal/integrations/tdarr"
	"github.com/z354prance/overmynd/internal/integrations/tracearr"
	"github.com/z354prance/overmynd/internal/services"
)

func main() {
	logger := slog.New(slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{
			Level: slog.LevelInfo,
		},
	))

	slog.SetDefault(logger)

	configDir := env(
		"OVERMYND_CONFIG_DIR",
		"/config",
	)

	addr := env(
		"OVERMYND_ADDR",
		":8080",
	)

	db, err := database.Open(configDir)
	if err != nil {
		slog.Error(
			"database initialization failed",
			"error", err,
		)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		slog.Error(
			"database migration failed",
			"error", err,
		)
		os.Exit(1)
	}

	slog.Info(
		"database ready",
		"path", db.Path(),
	)

	serviceManager := services.NewManager(db)

	authManager := auth.NewManager(db)

	if err := authManager.CleanupExpiredSessions(); err != nil {

		slog.Warn(

			"expired session cleanup failed",

			"error", err,
		)

	}

	registry := integrations.NewRegistry()

	for _, integration := range []integrations.Integration{
		radarr.New(),
		sonarr.New(),
		lidarr.New(),
		qbittorrent.New(),
		nzbget.New(),
		seerr.New(),
		tracearr.New(),
		tdarr.New(),
	} {
		if err := registry.Register(integration); err != nil {
			slog.Error(
				"integration registration failed",
				"error", err,
			)
			os.Exit(1)
		}
	}

	slog.Info(
		"integrations registered",
		"count", len(registry.Types()),
	)

	server := &http.Server{
		Addr: addr,

		Handler: api.NewRouter(
			serviceManager,
			registry,
			authManager,
		),

		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info(
			"starting Overmynd",
			"address", addr,
		)

		err := server.ListenAndServe()
		if err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			slog.Error(
				"server failed",
				"error", err,
			)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)

	signal.Notify(
		stop,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	<-stop

	slog.Info("shutting down Overmynd")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error(
			"graceful shutdown failed",
			"error", err,
		)
		os.Exit(1)
	}

	slog.Info("Overmynd stopped")
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
