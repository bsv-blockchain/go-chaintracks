// Package fiber provides Fiber route registration for chaintracks.
package fiber

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// NetworkResponse represents the response for the network endpoint
type NetworkResponse struct {
	Network string `json:"network" example:"mainnet"`
}

// HeightResponse represents the response for the height endpoint
type HeightResponse struct {
	Height uint32 `json:"height" example:"874123"`
}

// HeadersResponse represents the response for the headers endpoint
type HeadersResponse struct {
	Headers string `json:"headers" example:"0100000000000000..."`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error" example:"Header not found"`
}

// Routes handles HTTP routes for chaintracks.
type Routes struct {
	cm           chaintracks.Chaintracks
	sseClients   map[int64]*bufio.Writer
	sseClientsMu sync.RWMutex
	tipChan      <-chan *chaintracks.BlockHeader
}

// NewRoutes creates a new Routes instance.
func NewRoutes(cm chaintracks.Chaintracks) *Routes {
	return &Routes{
		cm:         cm,
		sseClients: make(map[int64]*bufio.Writer),
	}
}

// Register registers all chaintracks routes on the given router.
// Routes are registered at the root level of the provided router.
func (r *Routes) Register(router fiber.Router) {
	router.Get("/network", r.handleGetNetwork)
	router.Get("/height", r.handleGetHeight)
	router.Get("/tip", r.handleGetTip)
	router.Get("/tip/stream", r.handleTipStream)
	router.Get("/header/height/:height", r.handleGetHeaderByHeight)
	router.Get("/header/hash/:hash", r.handleGetHeaderByHash)
	router.Get("/headers", r.handleGetHeaders)
}

// StartBroadcasting starts broadcasting tip updates to SSE clients.
// Call this after starting the ChainManager to receive tip updates.
func (r *Routes) StartBroadcasting(ctx context.Context, tipChan <-chan *chaintracks.BlockHeader) {
	r.tipChan = tipChan
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tip, ok := <-tipChan:
				if !ok {
					return
				}
				if tip != nil {
					r.broadcastTip(tip)
				}
			}
		}
	}()
}

func (r *Routes) broadcastTip(tip *chaintracks.BlockHeader) {
	data, err := json.Marshal(tip)
	if err != nil {
		return
	}

	sseMessage := fmt.Sprintf("data: %s\n\n", string(data))

	r.sseClientsMu.RLock()
	clientsCopy := make(map[int64]*bufio.Writer, len(r.sseClients))
	for id, writer := range r.sseClients {
		clientsCopy[id] = writer
	}
	r.sseClientsMu.RUnlock()

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
		r.sseClientsMu.Lock()
		for _, id := range failedClients {
			delete(r.sseClients, id)
		}
		r.sseClientsMu.Unlock()
	}
}

// handleGetNetwork returns the network name
// @Summary Get network name
// @Description Returns the Bitcoin network this service is connected to
// @Tags chaintracks
// @Produce json
// @Success 200 {object} NetworkResponse
// @Failure 500 {object} ErrorResponse
// @Router /v2/network [get]
func (r *Routes) handleGetNetwork(c *fiber.Ctx) error {
	network, err := r.cm.GetNetwork(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"network": network})
}

// handleGetHeight returns the current chain height
// @Summary Get chain height
// @Description Returns the current blockchain height
// @Tags chaintracks
// @Produce json
// @Success 200 {object} HeightResponse
// @Router /v2/height [get]
func (r *Routes) handleGetHeight(c *fiber.Ctx) error {
	c.Set("Cache-Control", "public, max-age=60")
	return c.JSON(fiber.Map{"height": r.cm.GetHeight(c.UserContext())})
}

// handleGetTip returns the current chain tip
// @Summary Get chain tip
// @Description Returns the current chain tip block header
// @Tags chaintracks
// @Produce json
// @Success 200 {object} chaintracks.BlockHeader
// @Failure 404 {object} ErrorResponse
// @Router /v2/tip [get]
func (r *Routes) handleGetTip(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	tip := r.cm.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Chain tip not found"})
	}
	return c.JSON(tip)
}

// handleTipStream streams chain tip updates via SSE
// @Summary Stream chain tip updates
// @Description Server-Sent Events stream of chain tip updates. Sends the current tip immediately, then broadcasts new tips as they arrive.
// @Tags chaintracks
// @Produce text/event-stream
// @Success 200 {string} string "SSE stream of BlockHeader JSON objects"
// @Router /v2/tip/stream [get]
func (r *Routes) handleTipStream(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	ctx := c.UserContext()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		clientID := time.Now().UnixNano()

		r.sseClientsMu.Lock()
		r.sseClients[clientID] = w
		r.sseClientsMu.Unlock()

		defer func() {
			r.sseClientsMu.Lock()
			delete(r.sseClients, clientID)
			r.sseClientsMu.Unlock()
		}()

		// Send initial tip
		if tip := r.cm.GetTip(ctx); tip != nil {
			if data, err := json.Marshal(tip); err == nil {
				fmt.Fprintf(w, "data: %s\n\n", data)
				w.Flush()
			}
		}

		// Keep connection alive
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fmt.Fprintf(w, ": keepalive\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}

// handleGetHeaderByHeight returns a block header by height
// @Summary Get header by height
// @Description Returns a block header at the specified height
// @Tags chaintracks
// @Produce json
// @Param height path int true "Block height"
// @Success 200 {object} chaintracks.BlockHeader
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /v2/header/height/{height} [get]
func (r *Routes) handleGetHeaderByHeight(c *fiber.Ctx) error {
	height, err := strconv.ParseUint(c.Params("height"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid height parameter"})
	}

	ctx := c.UserContext()
	tip := r.cm.GetHeight(ctx)
	if uint32(height) < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	header, err := r.cm.GetHeaderByHeight(ctx, uint32(height))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Header not found"})
	}
	return c.JSON(header)
}

// handleGetHeaderByHash returns a block header by hash
// @Summary Get header by hash
// @Description Returns a block header with the specified hash
// @Tags chaintracks
// @Produce json
// @Param hash path string true "Block hash (hex)"
// @Success 200 {object} chaintracks.BlockHeader
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /v2/header/hash/{hash} [get]
func (r *Routes) handleGetHeaderByHash(c *fiber.Ctx) error {
	hash, err := chainhash.NewHashFromHex(c.Params("hash"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid hash parameter"})
	}

	ctx := c.UserContext()
	header, err := r.cm.GetHeaderByHash(ctx, hash)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Header not found"})
	}

	tip := r.cm.GetHeight(ctx)
	if header.Height < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	return c.JSON(header)
}

// handleGetHeaders returns multiple block headers as binary data
// @Summary Get multiple headers
// @Description Returns block headers starting from height as binary data (80 bytes per header)
// @Tags chaintracks
// @Produce application/octet-stream
// @Param height query int true "Starting block height"
// @Param count query int true "Number of headers to return"
// @Success 200 {string} binary "Concatenated 80-byte headers"
// @Failure 400 {object} ErrorResponse
// @Router /v2/headers [get]
func (r *Routes) handleGetHeaders(c *fiber.Ctx) error {
	heightStr := c.Query("height")
	countStr := c.Query("count")
	if heightStr == "" || countStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Missing height or count parameter"})
	}

	height, err := strconv.ParseUint(heightStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid height parameter"})
	}
	count, err := strconv.ParseUint(countStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid count parameter"})
	}

	ctx := c.UserContext()
	tip := r.cm.GetHeight(ctx)
	if uint32(height) < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	var data []byte
	for i := uint32(0); i < uint32(count); i++ {
		header, err := r.cm.GetHeaderByHeight(ctx, uint32(height)+i)
		if err != nil {
			break
		}
		data = append(data, header.Bytes()...)
	}

	c.Set("Content-Type", "application/octet-stream")
	return c.Send(data)
}
