package main

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// CDNServer serves static header files for CDN bootstrap.
type CDNServer struct {
	app         *fiber.App
	storagePath string
	port        int
}

// NewCDNServer creates a new CDN static file server.
func NewCDNServer(storagePath string, port int) *CDNServer {
	return &CDNServer{
		storagePath: storagePath,
		port:        port,
	}
}

// Start begins serving CDN files.
func (s *CDNServer) Start() error {
	s.app = fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          5 * time.Minute, // Large file transfers
	})

	// CORS for browser access
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "*",
		AllowMethods: "GET,HEAD,OPTIONS",
	}))

	// Set appropriate headers based on file type
	s.app.Use(func(c *fiber.Ctx) error {
		path := c.Path()
		ext := filepath.Ext(path)

		switch ext {
		case ".headers":
			c.Set("Content-Type", "application/octet-stream")
			c.Set("Cache-Control", "public, max-age=3600") // 1 hour cache for immutable header files
		case ".json":
			c.Set("Content-Type", "application/json")
			c.Set("Cache-Control", "no-cache") // Metadata should not be cached
		}

		return c.Next()
	})

	// Health check endpoint
	s.app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "cdn",
		})
	})

	// Serve static files from storage directory
	// Files served:
	//   /{network}NetBlockHeaders.json - Metadata JSON
	//   /{network}Net_X.headers        - Binary header files (100k headers each)
	s.app.Static("/", s.storagePath, fiber.Static{
		Compress:      true,
		ByteRange:     true, // Support Range requests for partial downloads
		Browse:        false,
		CacheDuration: 1 * time.Hour,
	})

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("CDN server starting on http://localhost%s", addr)
	log.Printf("  Serving files from: %s", s.storagePath)

	return s.app.Listen(addr)
}

// Shutdown gracefully stops the CDN server.
func (s *CDNServer) Shutdown() error {
	if s.app != nil {
		return s.app.Shutdown()
	}
	return nil
}
