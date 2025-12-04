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

	p2p "github.com/bsv-blockchain/go-p2p-message-bus"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"

	"github.com/bsv-blockchain/go-chaintracks/pkg/chaintracks"
)

func main() {
	_ = godotenv.Load()

	config := LoadConfig()
	logConfig(config)

	if err := ensureHeadersExist(config.StoragePath, config.Network); err != nil {
		log.Fatalf("Failed to initialize headers: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cm, err := createChainManager(ctx, config)
	if err != nil {
		log.Fatalf("Failed to create chain manager: %v", err)
	}

	logChainState(ctx, cm)

	blockMsgChan, err := cm.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start P2P: %v", err)
	}
	log.Printf("P2P listener started for network: %s", config.Network)

	app := createFiberApp(ctx, cm, blockMsgChan, config.Port)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	shutdown(cancel, cm, app)
}

func logConfig(config *Config) {
	log.Printf("Starting chaintracks-server")
	log.Printf("  Network: %s", config.Network)
	log.Printf("  Port: %d", config.Port)
	log.Printf("  Storage Path: %s", config.StoragePath)
	if config.BootstrapURL != "" {
		log.Printf("  Bootstrap URL: %s", config.BootstrapURL)
	}
}

func createChainManager(ctx context.Context, config *Config) (*chaintracks.ChainManager, error) {
	privKey, err := chaintracks.LoadOrGeneratePrivateKey(config.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load or generate private key: %w", err)
	}

	p2pClient, err := p2p.NewClient(p2p.Config{
		Name:          "go-chaintracks",
		Logger:        &p2p.DefaultLogger{},
		PrivateKey:    privKey,
		Port:          0,
		PeerCacheFile: filepath.Join(config.StoragePath, "peer_cache.json"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create P2P client: %w", err)
	}

	return chaintracks.NewChainManager(ctx, config.Network, config.StoragePath, p2pClient, config.BootstrapURL)
}

func logChainState(ctx context.Context, cm *chaintracks.ChainManager) {
	log.Printf("Loaded %d headers", cm.GetHeight(ctx))
	if tip := cm.GetTip(ctx); tip != nil {
		log.Printf("Chain tip: %s at height %d", tip.Header.Hash().String(), tip.Height)
	}
}

func createFiberApp(ctx context.Context, cm *chaintracks.ChainManager, blockMsgChan <-chan *chaintracks.BlockHeader, port int) *fiber.App {
	server := NewServer(cm)
	server.StartBroadcasting(ctx, blockMsgChan)

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
		log.Printf("  GET  http://localhost%s/v2/network - Network name", addr)
		log.Printf("  GET  http://localhost%s/v2/height - Current blockchain height", addr)
		log.Printf("  GET  http://localhost%s/v2/tip/header - Chain tip header", addr)
		log.Printf("  GET  http://localhost%s/v2/tip/stream - SSE stream for tip updates", addr)
		log.Printf("Press Ctrl+C to stop")

		if err := app.Listen(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	return app
}

func shutdown(cancel context.CancelFunc, cm *chaintracks.ChainManager, app *fiber.App) {
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
