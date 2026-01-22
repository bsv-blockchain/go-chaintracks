package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"

	"github.com/bsv-blockchain/go-chaintracks/chainmanager"
	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

func main() {
	_ = godotenv.Load()

	cfg, err := Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ct, err := cfg.Initialize(ctx)
	if err != nil {
		log.Fatalf("Failed to create chaintracks: %v", err)
	}

	go logStatus(ctx, ct)

	app := createFiberApp(ctx, ct, cfg.Port)

	// Start CDN server if enabled
	var cdnServer *CDNServer
	if cfg.CDNEnabled {
		cdnServer = NewCDNServer(cfg.Chaintracks.StoragePath, cfg.CDNPort)
		go func() {
			if err := cdnServer.Start(); err != nil {
				log.Printf("CDN server error: %v", err)
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	shutdown(cancel, ct, app, cdnServer)
}

func logConfig(cfg *AppConfig) {
	log.Printf("Starting chaintracks-server")
	log.Printf("  Network: %s", cfg.Chaintracks.P2P.Network)
	log.Printf("  API Port: %d", cfg.Port)
	if cfg.CDNEnabled {
		log.Printf("  CDN Port: %d", cfg.CDNPort)
	}
	log.Printf("  Storage Path: %s", cfg.Chaintracks.StoragePath)
	if cfg.Chaintracks.BootstrapURL != "" {
		log.Printf("  Bootstrap URL: %s (mode: %s)", cfg.Chaintracks.BootstrapURL, cfg.Chaintracks.BootstrapMode)
	}
}

func logStatus(ctx context.Context, ct chaintracks.Chaintracks) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Type-assert to get peer count if available
	var getPeerCount func() int
	if cm, ok := ct.(*chainmanager.ChainManager); ok {
		getPeerCount = func() int { return len(cm.P2PClient.GetPeers()) }
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tip := ct.GetTip(ctx)
			if tip != nil {
				if getPeerCount != nil {
					log.Printf("Height: %d, Tip: %s, Active Peers: %d", tip.Height, tip.Header.Hash().String(), getPeerCount())
				} else {
					log.Printf("Height: %d, Tip: %s", tip.Height, tip.Header.Hash().String())
				}
			}
		}
	}
}

func createFiberApp(ctx context.Context, ct chaintracks.Chaintracks, port int) *fiber.App {
	server := NewServer(ctx, ct)

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "*",
		AllowMethods: "GET,POST,OPTIONS",
	}))

	app.Use(logger.New(logger.Config{
		Format: "${method} ${path} - ${status} (${latency})\n",
	}))

	dashboard := NewDashboardHandler(server)
	server.SetupRoutes(app, dashboard)

	addr := fmt.Sprintf(":%d", port)
	go func() {
		log.Printf("Server listening on http://localhost%s", addr)
		log.Printf("Available endpoints:")
		log.Printf("  GET  http://localhost%s/ - Status Dashboard", addr)
		log.Printf("  GET  http://localhost%s/docs - API Documentation (Swagger UI)", addr)
		log.Printf("  GET  http://localhost%s/v2/network", addr)
		log.Printf("  GET  http://localhost%s/v2/tip", addr)
		log.Printf("  GET  http://localhost%s/v2/tip/stream", addr)
		log.Printf("  GET  http://localhost%s/v2/header/height/:height", addr)
		log.Printf("  GET  http://localhost%s/v2/header/hash/:hash", addr)
		log.Printf("  GET  http://localhost%s/v2/headers", addr)
		log.Printf("Press Ctrl+C to stop")

		if err := app.Listen(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	return app
}

func shutdown(cancel context.CancelFunc, ct chaintracks.Chaintracks, app *fiber.App, cdnServer *CDNServer) {
	log.Println("Shutting down gracefully...")
	cancel()
	// Close P2P client if this is an embedded ChainManager
	if cm, ok := ct.(*chainmanager.ChainManager); ok {
		if err := cm.P2PClient.Close(); err != nil {
			log.Printf("Error closing P2P client: %v", err)
		}
	}
	// Shutdown CDN server if running
	if cdnServer != nil {
		if err := cdnServer.Shutdown(); err != nil {
			log.Printf("Error closing CDN server: %v", err)
		}
	}
	if err := app.Shutdown(); err != nil {
		log.Printf("Error closing server: %v", err)
	}
	log.Println("Server stopped")
}
