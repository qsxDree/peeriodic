package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/logger"
	"github.com/logoes0/peeriodic.git/migrations"
	"github.com/logoes0/peeriodic.git/routers"
	"github.com/logoes0/peeriodic.git/services"
)

func main() {
	if err := run(); err != nil {
		logger.Error("Server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load(".env", "../.env")
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Init(cfg.Log.Format, cfg.Log.Level)
	db, err := services.NewDatabaseService(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	if cfg.Migrations.AutoMigrate {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = migrations.NewRunner(db.GetDB()).ApplyAll(ctx)
		cancel()
		if err != nil {
			return err
		}
	}
	auth := services.NewAuthService(cfg)
	ws := services.NewWebSocketService(cfg, auth)
	router := routers.NewRouter(db, ws, auth)
	server := &http.Server{
		Addr: cfg.GetServerAddress(), Handler: router.SetupRoutes(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	logger.Info("Server listening", "addr", server.Addr)
	select {
	case <-ctx.Done():
	case err = <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			err = fmt.Errorf("HTTP server: %w", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	wsErr := ws.Shutdown(shutdownCtx)
	httpErr := server.Shutdown(shutdownCtx)
	if httpErr != nil {
		_ = server.Close()
	}
	logger.Info("Server shutdown complete")
	return errors.Join(err, wsErr, httpErr)
}
