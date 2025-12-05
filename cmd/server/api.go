// Package main provides the ChainTracks HTTP API server.
package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/bsv-blockchain/go-chaintracks/pkg/chaintracks"
)

//go:embed openapi.yaml
var openapiSpec string

// Server wraps the ChainManager with Fiber handlers
type Server struct {
	ctx          context.Context
	cm           *chaintracks.ChainManager
	sseClients   map[int64]*bufio.Writer
	sseClientsMu sync.RWMutex
}

// NewServer creates a new API server
func NewServer(ctx context.Context, cm *chaintracks.ChainManager) *Server {
	return &Server{
		ctx:        ctx,
		cm:         cm,
		sseClients: make(map[int64]*bufio.Writer),
	}
}

// StartBroadcasting listens to ChainManager tip changes and broadcasts to all SSE clients
func (s *Server) StartBroadcasting(ctx context.Context, tipChan <-chan *chaintracks.BlockHeader) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tip := <-tipChan:
				if tip == nil {
					continue
				}
				s.broadcastTip(tip)
			}
		}
	}()
}

// broadcastTip sends a tip update to all connected SSE clients
func (s *Server) broadcastTip(tip *chaintracks.BlockHeader) {
	data, err := json.Marshal(tip)
	if err != nil {
		return
	}

	sseMessage := fmt.Sprintf("data: %s\n\n", string(data))

	s.sseClientsMu.RLock()
	clientsCopy := make(map[int64]*bufio.Writer, len(s.sseClients))
	for id, writer := range s.sseClients {
		clientsCopy[id] = writer
	}
	s.sseClientsMu.RUnlock()

	var failedClients []int64
	for id, writer := range clientsCopy {
		if _, err := fmt.Fprint(writer, sseMessage); err != nil {
			failedClients = append(failedClients, id)
			continue
		}
		if err := writer.Flush(); err != nil {
			failedClients = append(failedClients, id)
		}
	}

	if len(failedClients) > 0 {
		s.sseClientsMu.Lock()
		for _, id := range failedClients {
			delete(s.sseClients, id)
		}
		s.sseClientsMu.Unlock()
	}
}

// HandleTipStream handles SSE connections for tip updates
func (s *Server) HandleTipStream(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	// Capture context before entering stream writer
	ctx := c.UserContext()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		clientID := time.Now().UnixNano()

		s.sseClientsMu.Lock()
		s.sseClients[clientID] = w
		s.sseClientsMu.Unlock()

		defer func() {
			s.sseClientsMu.Lock()
			delete(s.sseClients, clientID)
			s.sseClientsMu.Unlock()
		}()

		// Send initial tip
		tip := s.cm.GetTip(ctx)
		if tip != nil { //nolint:nestif // SSE stream initialization logic

			data, err := json.Marshal(tip)
			if err == nil {
				if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", string(data)); writeErr != nil {
					return
				}
				if flushErr := w.Flush(); flushErr != nil {
					return
				}
			}
		}

		// Keep connection alive with periodic keepalive messages
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				if _, writeErr := fmt.Fprintf(w, ": keepalive\n\n"); writeErr != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}

// Response represents the standard API response format
type Response struct {
	Status      string      `json:"status"`
	Value       interface{} `json:"value,omitempty"`
	Code        string      `json:"code,omitempty"`
	Description string      `json:"description,omitempty"`
}

// HandleRoot returns service identification
func (s *Server) HandleRoot(c *fiber.Ctx) error {
	return c.JSON(Response{
		Status: "success",
		Value:  "chaintracks-server",
	})
}

// HandleRobots returns robots.txt
func (s *Server) HandleRobots(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/plain")
	return c.SendString("User-agent: *\nDisallow: /\n")
}

// HandleGetNetwork returns the network name
func (s *Server) HandleGetNetwork(c *fiber.Ctx) error {
	network, err := s.cm.GetNetwork(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(Response{
			Status: "error",
			Value:  err.Error(),
		})
	}
	return c.JSON(Response{
		Status: "success",
		Value:  network,
	})
}

// HandleGetHeight returns the current blockchain height
func (s *Server) HandleGetHeight(c *fiber.Ctx) error {
	c.Set("Cache-Control", "public, max-age=60")
	return c.JSON(Response{
		Status: "success",
		Value:  s.cm.GetHeight(c.UserContext()),
	})
}

// HandleGetTipHash returns the chain tip hash
func (s *Server) HandleGetTipHash(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")

	tip := s.cm.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(Response{
			Status:      "error",
			Code:        "ERR_NO_TIP",
			Description: "Chain tip not found",
		})
	}

	hash := tip.Header.Hash()
	return c.JSON(Response{
		Status: "success",
		Value:  &hash,
	})
}

// HandleGetTipHeader returns the full chain tip header
func (s *Server) HandleGetTipHeader(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")

	tip := s.cm.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(Response{
			Status:      "error",
			Code:        "ERR_NO_TIP",
			Description: "Chain tip not found",
		})
	}

	return c.JSON(Response{
		Status: "success",
		Value:  tip,
	})
}

// HandleGetHeaderByHeight returns a header by height
func (s *Server) HandleGetHeaderByHeight(c *fiber.Ctx) error {
	heightStr := c.Params("height")
	if heightStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(Response{
			Status:      "error",
			Code:        "ERR_INVALID_PARAMS",
			Description: "Missing height parameter",
		})
	}

	height, err := strconv.ParseUint(heightStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(Response{
			Status:      "error",
			Code:        "ERR_INVALID_PARAMS",
			Description: "Invalid height parameter",
		})
	}

	tip := s.cm.GetHeight(c.UserContext())
	if uint32(height) < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	header, err := s.cm.GetHeaderByHeight(c.UserContext(), uint32(height))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(Response{
			Status:      "error",
			Code:        "ERR_NOT_FOUND",
			Description: "Header not found at height " + heightStr,
		})
	}

	return c.JSON(Response{
		Status: "success",
		Value:  header,
	})
}

// HandleGetHeaderByHash returns a header by hash
func (s *Server) HandleGetHeaderByHash(c *fiber.Ctx) error {
	hashStr := c.Params("hash")
	if hashStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(Response{
			Status:      "error",
			Code:        "ERR_INVALID_PARAMS",
			Description: "Missing hash parameter",
		})
	}

	hash, err := chainhash.NewHashFromHex(hashStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(Response{
			Status:      "error",
			Code:        "ERR_INVALID_PARAMS",
			Description: "Invalid hash parameter",
		})
	}

	header, err := s.cm.GetHeaderByHash(c.UserContext(), hash)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(Response{
			Status:      "error",
			Code:        "ERR_NOT_FOUND",
			Description: "Header not found for hash " + hashStr,
		})
	}

	tip := s.cm.GetHeight(c.UserContext())
	if header.Height < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	return c.JSON(Response{
		Status: "success",
		Value:  header,
	})
}

// HandleGetHeaders returns multiple headers as concatenated hex
func (s *Server) HandleGetHeaders(c *fiber.Ctx) error {
	heightStr := c.Query("height")
	countStr := c.Query("count")

	if heightStr == "" || countStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(Response{
			Status:      "error",
			Code:        "ERR_INVALID_PARAMS",
			Description: "Missing height or count parameter",
		})
	}

	height, err := strconv.ParseUint(heightStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(Response{
			Status:      "error",
			Code:        "ERR_INVALID_PARAMS",
			Description: "Invalid height parameter",
		})
	}

	count, err := strconv.ParseUint(countStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(Response{
			Status:      "error",
			Code:        "ERR_INVALID_PARAMS",
			Description: "Invalid count parameter",
		})
	}

	tip := s.cm.GetHeight(c.UserContext())
	if uint32(height) < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	var hexData string
	for i := uint32(0); i < uint32(count); i++ {
		h := uint32(height) + i
		header, err := s.cm.GetHeaderByHeight(c.UserContext(), h)
		if err != nil {
			break
		}

		headerBytes := header.Bytes()
		hexData += hex.EncodeToString(headerBytes)
	}

	return c.JSON(Response{
		Status: "success",
		Value:  hexData,
	})
}

// HandleOpenAPISpec serves the OpenAPI specification
func (s *Server) HandleOpenAPISpec(c *fiber.Ctx) error {
	c.Set("Content-Type", "application/yaml")
	return c.SendString(openapiSpec)
}

// HandleSwaggerUI serves the Swagger UI
func (s *Server) HandleSwaggerUI(c *fiber.Ctx) error {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Chaintracks API Documentation</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.10.0/swagger-ui.css">
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.10.0/swagger-ui-bundle.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: '/openapi.yaml',
                dom_id: '#swagger-ui',
                deepLinking: true,
                tryItOutEnabled: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIBundle.SwaggerUIStandalonePreset
                ]
            });
        };
    </script>
</body>
</html>`
	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

// SetupRoutes configures all Fiber routes
func (s *Server) SetupRoutes(app *fiber.App, dashboard *DashboardHandler) {
	app.Get("/", dashboard.HandleStatus)
	app.Get("/robots.txt", s.HandleRobots)
	app.Get("/docs", s.HandleSwaggerUI)
	app.Get("/openapi.yaml", s.HandleOpenAPISpec)

	v2 := app.Group("/v2")
	v2.Get("/network", s.HandleGetNetwork)
	v2.Get("/height", s.HandleGetHeight)
	v2.Get("/tip/hash", s.HandleGetTipHash)
	v2.Get("/tip/header", s.HandleGetTipHeader)
	v2.Get("/tip/stream", s.HandleTipStream)
	v2.Get("/header/height/:height", s.HandleGetHeaderByHeight)
	v2.Get("/header/hash/:hash", s.HandleGetHeaderByHash)
	v2.Get("/headers", s.HandleGetHeaders)
}
