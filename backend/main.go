package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/routers"
	"github.com/logoes0/peeriodic.git/services"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env file not found, using system environment variables")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Failed to load configuration: %v", err)
	}

	// Initialize database service
	dbService, err := services.NewDatabaseService(cfg)
	if err != nil {
		log.Fatalf("❌ Failed to initialize database service: %v", err)
	}
	defer func() {
		if err := dbService.Close(); err != nil {
			log.Printf("⚠️  Failed to close database connection: %v", err)
		}
	}()

	// Initialize auth service
	authService := services.NewAuthService(cfg)

	// Initialize WebSocket service
	wsService := services.NewWebSocketService(cfg, authService)

	// Initialize router
	router := routers.NewRouter(dbService, wsService, authService)
	router.SetupRoutes()

	// Create HTTP server
	server := &http.Server{
		Addr:         cfg.GetServerAddress(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("🚀 Server starting on %s", cfg.GetServerAddress())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down server...")

	// Create a deadline for server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("⚠️  Server forced to shutdown: %v", err)
	}

	log.Println("✅ Server exited gracefully")
}
