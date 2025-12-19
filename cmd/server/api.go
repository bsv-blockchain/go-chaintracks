// Package main provides the ChainTracks HTTP API server.
package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

// HandleGetTipBinary returns the chain tip as 80-byte binary with height in X-Block-Height header
func (s *Server) HandleGetTipBinary(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	c.Set("Content-Type", "application/octet-stream")

	tip := s.ct.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(Response{
			Status:      "error",
			Code:        "ERR_NO_TIP",
			Description: "Chain tip not found",
		})
	}

	c.Set("X-Block-Height", strconv.FormatUint(uint64(tip.Height), 10))
	return c.Send(tip.Bytes())
}

// HandleGetHeaderByHeightBinary returns a header by height as 80-byte binary with height in X-Block-Height header
func (s *Server) HandleGetHeaderByHeightBinary(c *fiber.Ctx) error {
	heightStr := c.Params("height")
	// Remove .bin suffix if present in the param
	heightStr = strings.TrimSuffix(heightStr, ".bin")
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

	c.Set("Content-Type", "application/octet-stream")
	c.Set("X-Block-Height", strconv.FormatUint(uint64(header.Height), 10))
	return c.Send(header.Bytes())
}

// HandleGetHeaderByHashBinary returns a header by hash as 80-byte binary with height in X-Block-Height header
func (s *Server) HandleGetHeaderByHashBinary(c *fiber.Ctx) error {
	hashStr := c.Params("hash")
	// Remove .bin suffix if present in the param
	hashStr = strings.TrimSuffix(hashStr, ".bin")
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

	c.Set("Content-Type", "application/octet-stream")
	c.Set("X-Block-Height", strconv.FormatUint(uint64(header.Height), 10))
	return c.Send(header.Bytes())
}

// HandleGetHeadersBinary returns multiple headers as binary (80 bytes each)
// X-Start-Height header contains the starting height, headers are sequential from there
func (s *Server) HandleGetHeadersBinary(c *fiber.Ctx) error {
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
	var headerCount uint32
	for i := uint32(0); i < uint32(count); i++ {
		header, err := s.ct.GetHeaderByHeight(c.UserContext(), uint32(height)+i)
		if err != nil {
			break
		}
		data = append(data, header.Bytes()...)
		headerCount++
	}

	c.Set("Content-Type", "application/octet-stream")
	c.Set("X-Start-Height", strconv.FormatUint(height, 10))
	c.Set("X-Header-Count", strconv.FormatUint(uint64(headerCount), 10))
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

	// V2 routes (RESTful API)
	v2 := app.Group("/v2")
	v2.Get("/network", s.HandleGetNetwork)
	v2.Get("/tip", s.HandleGetTip)
	v2.Get("/tip/stream", s.HandleTipStream)
	v2.Get("/header/height/:height", s.HandleGetHeaderByHeight)
	v2.Get("/header/hash/:hash", s.HandleGetHeaderByHash)
	v2.Get("/headers", s.HandleGetHeaders)

	// V2 binary routes (84 bytes per header: 4-byte height LE + 80-byte header)
	v2.Get("/tip.bin", s.HandleGetTipBinary)
	v2.Get("/header/height/:height.bin", s.HandleGetHeaderByHeightBinary)
	v2.Get("/header/hash/:hash.bin", s.HandleGetHeaderByHashBinary)
	v2.Get("/headers.bin", s.HandleGetHeadersBinary)

	// Legacy v1 routes (RPC-style API for backwards compatibility)
	s.SetupLegacyRoutes(app)
}

// LegacyResponse represents the standard v1 API response format
type LegacyResponse struct {
	Status      string      `json:"status"`
	Value       interface{} `json:"value,omitempty"`
	Code        string      `json:"code,omitempty"`
	Description string      `json:"description,omitempty"`
}

func legacySuccess(value interface{}) LegacyResponse {
	return LegacyResponse{Status: "success", Value: value}
}

func legacyError(code, description string) LegacyResponse {
	return LegacyResponse{Status: "error", Code: code, Description: description}
}

// SetupLegacyRoutes configures v1-compatible routes (RPC-style endpoints)
func (s *Server) SetupLegacyRoutes(app *fiber.App) {
	app.Get("/getChain", s.HandleLegacyGetChain)
	app.Get("/getPresentHeight", s.HandleLegacyGetPresentHeight)
	app.Get("/findChainTipHashHex", s.HandleLegacyFindChainTipHashHex)
	app.Get("/findChainTipHeaderHex", s.HandleLegacyFindChainTipHeaderHex)
	app.Get("/findHeaderHexForHeight", s.HandleLegacyFindHeaderHexForHeight)
	app.Get("/findHeaderHexForBlockHash", s.HandleLegacyFindHeaderHexForBlockHash)
	app.Get("/getHeaders", s.HandleLegacyGetHeaders)
}

// HandleLegacyGetChain returns the network name in legacy format
func (s *Server) HandleLegacyGetChain(c *fiber.Ctx) error {
	network, err := s.ct.GetNetwork(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(legacyError("ERR_INTERNAL", err.Error()))
	}
	return c.JSON(legacySuccess(network))
}

// HandleLegacyGetPresentHeight returns the current chain height in legacy format
func (s *Server) HandleLegacyGetPresentHeight(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	return c.JSON(legacySuccess(s.ct.GetHeight(c.UserContext())))
}

// HandleLegacyFindChainTipHashHex returns just the chain tip hash
func (s *Server) HandleLegacyFindChainTipHashHex(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	tip := s.ct.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(legacyError("ERR_NO_TIP", "Chain tip not found"))
	}
	return c.JSON(legacySuccess(tip.Hash.String()))
}

// HandleLegacyFindChainTipHeaderHex returns the chain tip header in legacy format
func (s *Server) HandleLegacyFindChainTipHeaderHex(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	tip := s.ct.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(legacyError("ERR_NO_TIP", "Chain tip not found"))
	}
	return c.JSON(legacySuccess(tip))
}

// HandleLegacyFindHeaderHexForHeight returns a header by height using query param
func (s *Server) HandleLegacyFindHeaderHexForHeight(c *fiber.Ctx) error {
	heightStr := c.Query("height")
	if heightStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Missing height parameter"))
	}

	height, err := strconv.ParseUint(heightStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Invalid height parameter"))
	}

	ctx := c.UserContext()
	tip := s.ct.GetHeight(ctx)
	if uint32(height) < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	header, err := s.ct.GetHeaderByHeight(ctx, uint32(height))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(legacyError("ERR_NOT_FOUND", "Header not found at height "+heightStr))
	}
	return c.JSON(legacySuccess(header))
}

// HandleLegacyFindHeaderHexForBlockHash returns a header by hash using query param
func (s *Server) HandleLegacyFindHeaderHexForBlockHash(c *fiber.Ctx) error {
	hashStr := c.Query("hash")
	if hashStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Missing hash parameter"))
	}

	hash, err := chainhash.NewHashFromHex(hashStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Invalid hash parameter"))
	}

	ctx := c.UserContext()
	header, err := s.ct.GetHeaderByHash(ctx, hash)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(legacyError("ERR_NOT_FOUND", "Header not found for hash "+hashStr))
	}

	tip := s.ct.GetHeight(ctx)
	if header.Height < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	return c.JSON(legacySuccess(header))
}

// HandleLegacyGetHeaders returns multiple headers as hex string in legacy format
func (s *Server) HandleLegacyGetHeaders(c *fiber.Ctx) error {
	heightStr := c.Query("height")
	countStr := c.Query("count")
	if heightStr == "" || countStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Missing height or count parameter"))
	}

	height, err := strconv.ParseUint(heightStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Invalid height parameter"))
	}
	count, err := strconv.ParseUint(countStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Invalid count parameter"))
	}

	ctx := c.UserContext()
	tip := s.ct.GetHeight(ctx)
	if uint32(height) < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	var data []byte
	for i := uint32(0); i < uint32(count); i++ {
		header, err := s.ct.GetHeaderByHeight(ctx, uint32(height)+i)
		if err != nil {
			break
		}
		data = append(data, header.Bytes()...)
	}

	// Return as hex string wrapped in legacy response
	return c.JSON(legacySuccess(fmt.Sprintf("%x", data)))
}
