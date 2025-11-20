package main

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/gofiber/fiber/v2"

	"github.com/bsv-blockchain/go-chaintracks/pkg/chaintracks"
)

func setupTestApp(t *testing.T) (*fiber.App, *chaintracks.ChainManager) {
	cm, err := chaintracks.NewChainManager("main", "../../data/headers", "")
	if err != nil {
		t.Fatalf("Failed to create chain manager: %v", err)
	}

	server := NewServer(cm)
	app := fiber.New()

	dashboard := NewDashboardHandler(server)
	server.SetupRoutes(app, dashboard)

	return app, cm
}

func TestHandleGetNetwork(t *testing.T) {
	app, _ := setupTestApp(t)

	req := httptest.NewRequest("GET", "/v2/network", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Value != "main" {
		t.Errorf("Expected value 'main', got '%v'", response.Value)
	}
}

func TestHandleGetHeight(t *testing.T) {
	app, cm := setupTestApp(t)

	req := httptest.NewRequest("GET", "/v2/height", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	cacheControl := resp.Header.Get("Cache-Control")
	if cacheControl != "public, max-age=60" {
		t.Errorf("Expected Cache-Control 'public, max-age=60', got '%s'", cacheControl)
	}

	body, _ := io.ReadAll(resp.Body)
	var response struct {
		Status string  `json:"status"`
		Value  float64 `json:"value"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	expectedHeight := cm.GetHeight()
	if uint32(response.Value) != expectedHeight {
		t.Errorf("Expected height %d, got %d", expectedHeight, uint32(response.Value))
	}
}

func TestHandleGetTipHash(t *testing.T) {
	app, cm := setupTestApp(t)

	req := httptest.NewRequest("GET", "/v2/tip/hash", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	cacheControl := resp.Header.Get("Cache-Control")
	if cacheControl != "no-cache" {
		t.Errorf("Expected Cache-Control 'no-cache', got '%s'", cacheControl)
	}

	body, _ := io.ReadAll(resp.Body)
	var response struct {
		Status string `json:"status"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	tip := cm.GetTip()
	expectedHash := tip.Header.Hash().String()
	if response.Value != expectedHash {
		t.Errorf("Expected hash %s, got %s", expectedHash, response.Value)
	}
}

func TestHandleGetTipHeader(t *testing.T) {
	app, cm := setupTestApp(t)

	req := httptest.NewRequest("GET", "/v2/tip/header", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	tip := cm.GetTip()
	if response.Value.Height != tip.Height {
		t.Errorf("Expected height %d, got %d", tip.Height, response.Value.Height)
	}
}

func TestHandleGetHeaderByHeight(t *testing.T) {
	app, cm := setupTestApp(t)

	height := cm.GetHeight()
	if height < 100 {
		t.Skip("Not enough headers to test")
	}

	req := httptest.NewRequest("GET", "/v2/header/height/100", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Value.Height != 100 {
		t.Errorf("Expected height 100, got %d", response.Value.Height)
	}
}

func TestHandleGetHeaderByHeight_NotFound(t *testing.T) {
	app, _ := setupTestApp(t)

	req := httptest.NewRequest("GET", "/v2/header/height/99999999", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var response struct {
		Status string      `json:"status"`
		Value  interface{} `json:"value"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Value != nil {
		t.Errorf("Expected null value for non-existent header, got %v", response.Value)
	}
}

func TestHandleGetHeaderByHash(t *testing.T) {
	app, cm := setupTestApp(t)

	tip := cm.GetTip()
	hash := tip.Header.Hash().String()

	req := httptest.NewRequest("GET", "/v2/header/hash/"+hash, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Value.Height != tip.Height {
		t.Errorf("Expected height %d, got %d", tip.Height, response.Value.Height)
	}
}

func TestHandleGetHeaderByHash_InvalidHash(t *testing.T) {
	app, _ := setupTestApp(t)

	req := httptest.NewRequest("GET", "/v2/header/hash/invalid", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 400 {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", response.Status)
	}
}

func TestHandleGetHeaderByHash_NotFound(t *testing.T) {
	app, _ := setupTestApp(t)

	nonExistentHash := chainhash.Hash{}
	req := httptest.NewRequest("GET", "/v2/header/hash/"+nonExistentHash.String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var response struct {
		Status string      `json:"status"`
		Value  interface{} `json:"value"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Value != nil {
		t.Errorf("Expected null value for non-existent header, got %v", response.Value)
	}
}

func TestHandleGetHeaders(t *testing.T) {
	app, cm := setupTestApp(t)

	height := cm.GetHeight()
	if height < 10 {
		t.Skip("Not enough headers to test")
	}

	req := httptest.NewRequest("GET", "/v2/headers?height=0&count=10", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var response struct {
		Status string `json:"status"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	expectedLen := 10 * 80 * 2
	if len(response.Value) != expectedLen {
		t.Errorf("Expected hex string length %d (10 headers * 80 bytes * 2), got %d", expectedLen, len(response.Value))
	}
}

func TestHandleGetHeaders_MissingParams(t *testing.T) {
	app, _ := setupTestApp(t)

	req := httptest.NewRequest("GET", "/v2/headers?height=0", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 400 {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", response.Status)
	}
}
