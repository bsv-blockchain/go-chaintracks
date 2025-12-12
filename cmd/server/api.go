// Package main provides the ChainTracks HTTP API server.
package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

//go:embed openapi.yaml
var openapiSpec string

// Server wraps the Chaintracks interface with Fiber handlers
//
//nolint:containedctx // Context stored for SSE stream shutdown detection
type Server struct {
	ctx context.Context
	ct  chaintracks.Chaintracks
}

// NewServer creates a new API server
func NewServer(ctx context.Context, ct chaintracks.Chaintracks) *Server {
	return &Server{
		ctx: ctx,
		ct:  ct,
	}
}

// HandleTipStream handles SSE connections for tip updates
func (s *Server) HandleTipStream(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		// Create a context that cancels when the client disconnects
		ctx, cancel := context.WithCancel(s.ctx)
		defer cancel()

		// Subscribe to tip updates
		tipChan := s.ct.Subscribe(ctx)

		// Send initial tip
		if tip := s.ct.GetTip(ctx); tip != nil {
			if data, err := json.Marshal(tip); err == nil {
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
			case <-ctx.Done():
				return
			case tip := <-tipChan:
				if tip == nil {
					continue
				}
				if data, err := json.Marshal(tip); err == nil {
					if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", string(data)); writeErr != nil {
						return
					}
					if flushErr := w.Flush(); flushErr != nil {
						return
					}
				}
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
	network, err := s.ct.GetNetwork(c.UserContext())
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

// HandleGetTip returns the chain tip header
func (s *Server) HandleGetTip(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")

	tip := s.ct.GetTip(c.UserContext())
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

	tip := s.ct.GetHeight(c.UserContext())
	if uint32(height) < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	header, err := s.ct.GetHeaderByHeight(c.UserContext(), uint32(height))
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

	header, err := s.ct.GetHeaderByHash(c.UserContext(), hash)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(Response{
			Status:      "error",
			Code:        "ERR_NOT_FOUND",
			Description: "Header not found for hash " + hashStr,
		})
	}

	tip := s.ct.GetHeight(c.UserContext())
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

	tip := s.ct.GetHeight(c.UserContext())
	if uint32(height) < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	var data []byte
	for i := uint32(0); i < uint32(count); i++ {
		header, err := s.ct.GetHeaderByHeight(c.UserContext(), uint32(height)+i)
		if err != nil {
			break
		}
		data = append(data, header.Bytes()...)
	}

	c.Set("Content-Type", "application/octet-stream")
	return c.Send(data)
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
	v2.Get("/tip", s.HandleGetTip)
	v2.Get("/tip/stream", s.HandleTipStream)
	v2.Get("/header/height/:height", s.HandleGetHeaderByHeight)
	v2.Get("/header/hash/:hash", s.HandleGetHeaderByHash)
	v2.Get("/headers", s.HandleGetHeaders)
}
