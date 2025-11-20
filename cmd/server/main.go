package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"

	"github.com/bsv-blockchain/go-chaintracks/pkg/chaintracks"
)

func main() {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	config := LoadConfig()

	log.Printf("Starting chaintracks-server")
	log.Printf("  Network: %s", config.Network)
	log.Printf("  Port: %d", config.Port)
	log.Printf("  Storage Path: %s", config.StoragePath)
	if config.BootstrapURL != "" {
		log.Printf("  Bootstrap URL: %s", config.BootstrapURL)
	}

	if err := ensureHeadersExist(config.StoragePath, config.Network); err != nil {
		log.Fatalf("Failed to initialize headers: %v", err)
	}

	// Create chain manager with optional bootstrap URL
	// Bootstrap happens synchronously in the constructor before returning
	cm, err := chaintracks.NewChainManager(config.Network, config.StoragePath, config.BootstrapURL)
	if err != nil {
		log.Fatalf("Failed to create chain manager: %v", err)
	}

	height := cm.GetHeight()
	log.Printf("Loaded %d headers", height)

	if tip := cm.GetTip(); tip != nil {
		log.Printf("Chain tip: %s at height %d", tip.Header.Hash().String(), tip.Height)
	}

	// Start P2P listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockMsgChan, err := cm.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start P2P: %v", err)
	}
	log.Printf("P2P listener started for network: %s", config.Network)

	server := NewServer(cm)

	// Start broadcasting tip changes to SSE clients
	server.StartBroadcasting(ctx, blockMsgChan)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Add middleware
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "*",
		AllowMethods: "GET,POST,OPTIONS",
	}))

	app.Use(logger.New(logger.Config{
		Format: "${method} ${path} - ${status} (${latency})\n",
	}))

	// Create dashboard
	dashboard := NewDashboardHandler(server)

	// Setup routes
	server.SetupRoutes(app, dashboard)

	addr := fmt.Sprintf(":%d", config.Port)

	go func() {
		log.Printf("Server listening on http://localhost%s", addr)
		log.Printf("Available endpoints:")
		log.Printf("  GET  http://localhost%s/ - Status Dashboard", addr)
		log.Printf("  GET  http://localhost%s/docs - API Documentation (Swagger UI)", addr)
		log.Printf("  GET  http://localhost%s/v2/network - Network name", addr)
		log.Printf("  GET  http://localhost%s/v2/height - Current blockchain height", addr)
		log.Printf("  GET  http://localhost%s/v2/tip/header - Chain tip header", addr)
		log.Printf("  GET  http://localhost%s/v2/tip/stream - SSE stream for tip updates", addr)
		log.Printf("Press Ctrl+C to stop")

		if err := app.Listen(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down gracefully...")
	cancel()
	if err := cm.Stop(); err != nil {
		log.Printf("Error closing P2P: %v", err)
	}
	if err := app.Shutdown(); err != nil {
		log.Printf("Error closing server: %v", err)
	}
	log.Println("Server stopped")
}

// ensureHeadersExist checks if headers exist at storagePath, and if not, copies from checkpoint
func ensureHeadersExist(storagePath, network string) error {
	metadataFile := filepath.Join(storagePath, network+"NetBlockHeaders.json")

	if _, err := os.Stat(metadataFile); err == nil {
		return nil
	}

	log.Printf("No headers found at %s, initializing from checkpoint...", storagePath)

	checkpointPath := filepath.Join("data", "headers")
	checkpointMetadata := filepath.Join(checkpointPath, network+"NetBlockHeaders.json")

	if _, err := os.Stat(checkpointMetadata); os.IsNotExist(err) {
		log.Printf("Warning: No checkpoint headers found at %s", checkpointPath)
		return nil
	}

	if err := os.MkdirAll(storagePath, 0o750); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(checkpointPath, network+"*"))
	if err != nil {
		return fmt.Errorf("failed to list checkpoint files: %w", err)
	}

	log.Printf("Copying %d checkpoint files to %s...", len(files), storagePath)
	for _, srcFile := range files {
		dstFile := filepath.Join(storagePath, filepath.Base(srcFile))
		if err := copyFile(srcFile, dstFile); err != nil {
			return fmt.Errorf("failed to copy %s: %w", srcFile, err)
		}
	}

	log.Printf("Checkpoint headers initialized successfully")
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) (err error) {
	sourceFile, err := os.Open(src) //nolint:gosec // Source path is from embedded checkpoint files
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := sourceFile.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	destFile, err := os.Create(dst) //nolint:gosec // Destination path is validated
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := destFile.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}
