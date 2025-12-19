// Package fiber provides Fiber route registration for chaintracks.
package fiber

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

// NewRoutes creates a new Routes instance and starts broadcasting tip updates to SSE clients.
// The context is used for cancellation - when cancelled, the broadcast goroutine will stop.
func NewRoutes(ctx context.Context, cm chaintracks.Chaintracks) *Routes {
	r := &Routes{
		cm:         cm,
		sseClients: make(map[int64]*bufio.Writer),
	}

	tipChan := cm.Subscribe(ctx)
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

	return r
}

// Register registers all chaintracks routes on the given router.
// Routes are registered at the root level of the provided router.
func (r *Routes) Register(router fiber.Router) {
	// JSON routes
	router.Get("/network", r.handleGetNetwork)
	router.Get("/height", r.handleGetHeight)
	router.Get("/tip", r.handleGetTip)
	router.Get("/tip/stream", r.handleTipStream)
	router.Get("/header/height/:height", r.handleGetHeaderByHeight)
	router.Get("/header/hash/:hash", r.handleGetHeaderByHash)
	router.Get("/headers", r.handleGetHeaders)

	// Binary routes (84 bytes per header: 4-byte height LE + 80-byte header)
	router.Get("/tip.bin", r.handleGetTipBinary)
	router.Get("/header/height/:height.bin", r.handleGetHeaderByHeightBinary)
	router.Get("/header/hash/:hash.bin", r.handleGetHeaderByHashBinary)
	router.Get("/headers.bin", r.handleGetHeadersBinary)
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

// RegisterLegacy registers v1-compatible routes (RPC-style endpoints)
// matching the original chaintracks-server API format.
func (r *Routes) RegisterLegacy(router fiber.Router) {
	router.Get("/getChain", r.handleLegacyGetChain)
	router.Get("/getPresentHeight", r.handleLegacyGetPresentHeight)
	router.Get("/findChainTipHashHex", r.handleLegacyFindChainTipHashHex)
	router.Get("/findChainTipHeaderHex", r.handleLegacyFindChainTipHeaderHex)
	router.Get("/findHeaderHexForHeight", r.handleLegacyFindHeaderHexForHeight)
	router.Get("/findHeaderHexForBlockHash", r.handleLegacyFindHeaderHexForBlockHash)
	router.Get("/getHeaders", r.handleLegacyGetHeaders)
}

// handleLegacyGetChain returns the network name in legacy format
func (r *Routes) handleLegacyGetChain(c *fiber.Ctx) error {
	network, err := r.cm.GetNetwork(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(legacyError("ERR_INTERNAL", err.Error()))
	}
	return c.JSON(legacySuccess(network))
}

// handleLegacyGetPresentHeight returns the current chain height in legacy format
func (r *Routes) handleLegacyGetPresentHeight(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	return c.JSON(legacySuccess(r.cm.GetHeight(c.UserContext())))
}

// handleLegacyFindChainTipHashHex returns just the chain tip hash
func (r *Routes) handleLegacyFindChainTipHashHex(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	tip := r.cm.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(legacyError("ERR_NO_TIP", "Chain tip not found"))
	}
	return c.JSON(legacySuccess(tip.Hash.String()))
}

// handleLegacyFindChainTipHeaderHex returns the chain tip header in legacy format
func (r *Routes) handleLegacyFindChainTipHeaderHex(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	tip := r.cm.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(legacyError("ERR_NO_TIP", "Chain tip not found"))
	}
	return c.JSON(legacySuccess(tip))
}

// handleLegacyFindHeaderHexForHeight returns a header by height using query param
func (r *Routes) handleLegacyFindHeaderHexForHeight(c *fiber.Ctx) error {
	heightStr := c.Query("height")
	if heightStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Missing height parameter"))
	}

	height, err := strconv.ParseUint(heightStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Invalid height parameter"))
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
		return c.Status(fiber.StatusNotFound).JSON(legacyError("ERR_NOT_FOUND", "Header not found at height "+heightStr))
	}
	return c.JSON(legacySuccess(header))
}

// handleLegacyFindHeaderHexForBlockHash returns a header by hash using query param
func (r *Routes) handleLegacyFindHeaderHexForBlockHash(c *fiber.Ctx) error {
	hashStr := c.Query("hash")
	if hashStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Missing hash parameter"))
	}

	hash, err := chainhash.NewHashFromHex(hashStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(legacyError("ERR_INVALID_PARAMS", "Invalid hash parameter"))
	}

	ctx := c.UserContext()
	header, err := r.cm.GetHeaderByHash(ctx, hash)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(legacyError("ERR_NOT_FOUND", "Header not found for hash "+hashStr))
	}

	tip := r.cm.GetHeight(ctx)
	if header.Height < tip-100 {
		c.Set("Cache-Control", "public, max-age=3600")
	} else {
		c.Set("Cache-Control", "no-cache")
	}

	return c.JSON(legacySuccess(header))
}

// handleLegacyGetHeaders returns multiple headers as hex string in legacy format
func (r *Routes) handleLegacyGetHeaders(c *fiber.Ctx) error {
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

	// Return as hex string wrapped in legacy response
	return c.JSON(legacySuccess(fmt.Sprintf("%x", data)))
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
// @Router /chaintracks/network [get]
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
// @Router /chaintracks/height [get]
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
// @Router /chaintracks/tip [get]
func (r *Routes) handleGetTip(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	tip := r.cm.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Chain tip not found"})
	}
	c.Set("X-Block-Height", strconv.FormatUint(uint64(tip.Height), 10))
	return c.JSON(tip)
}

// handleTipStream streams chain tip updates via SSE
// @Summary Stream chain tip updates
// @Description Server-Sent Events stream of chain tip updates. Sends the current tip immediately, then broadcasts new tips as they arrive.
// @Tags chaintracks
// @Produce text/event-stream
// @Success 200 {string} string "SSE stream of BlockHeader JSON objects"
// @Router /chaintracks/tip/stream [get]
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
// @Router /chaintracks/header/height/{height} [get]
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
// @Router /chaintracks/header/hash/{hash} [get]
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
// @Router /chaintracks/headers [get]
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

// Binary handlers (80 bytes per header, height returned in X-Block-Height header)

// handleGetTipBinary returns the chain tip as 80-byte binary
func (r *Routes) handleGetTipBinary(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache")
	c.Set("Content-Type", "application/octet-stream")

	tip := r.cm.GetTip(c.UserContext())
	if tip == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Chain tip not found"})
	}

	c.Set("X-Block-Height", strconv.FormatUint(uint64(tip.Height), 10))
	return c.Send(tip.Bytes())
}

// handleGetHeaderByHeightBinary returns a header by height as 80-byte binary
func (r *Routes) handleGetHeaderByHeightBinary(c *fiber.Ctx) error {
	heightStr := c.Params("height")
	heightStr = strings.TrimSuffix(heightStr, ".bin")

	height, err := strconv.ParseUint(heightStr, 10, 32)
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

	c.Set("Content-Type", "application/octet-stream")
	c.Set("X-Block-Height", strconv.FormatUint(uint64(header.Height), 10))
	return c.Send(header.Bytes())
}

// handleGetHeaderByHashBinary returns a header by hash as 80-byte binary
func (r *Routes) handleGetHeaderByHashBinary(c *fiber.Ctx) error {
	hashStr := c.Params("hash")
	hashStr = strings.TrimSuffix(hashStr, ".bin")

	hash, err := chainhash.NewHashFromHex(hashStr)
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

	c.Set("Content-Type", "application/octet-stream")
	c.Set("X-Block-Height", strconv.FormatUint(uint64(header.Height), 10))
	return c.Send(header.Bytes())
}

// handleGetHeadersBinary returns multiple headers as binary (80 bytes each)
func (r *Routes) handleGetHeadersBinary(c *fiber.Ctx) error {
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
	var headerCount uint32
	for i := uint32(0); i < uint32(count); i++ {
		header, err := r.cm.GetHeaderByHeight(ctx, uint32(height)+i)
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
