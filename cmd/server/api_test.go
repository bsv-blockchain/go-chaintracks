package main

import (
	"strings"
	"testing"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-chaintracks/pkg/chaintracks"
)

func TestHandleGetNetwork(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/v2/network")
	requireStatus(t, resp, 200)

	response := requireSuccessResponse(t, resp.Body)
	assert.Equal(t, "main", response.Value)
}

func TestHandleGetHeight(t *testing.T) {
	app, cm := setupTestApp(t)

	resp := httpGet(t, app, "/v2/height")
	requireStatus(t, resp, 200)
	assert.Equal(t, "public, max-age=60", resp.Headers["Cache-Control"])

	var response struct {
		Status string  `json:"status"`
		Value  float64 `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	assert.Equal(t, cm.GetHeight(), uint32(response.Value))
}

func TestHandleGetTipHash(t *testing.T) {
	app, cm := setupTestApp(t)

	resp := httpGet(t, app, "/v2/tip/hash")
	requireStatus(t, resp, 200)
	assert.Equal(t, "no-cache", resp.Headers["Cache-Control"])

	var response struct {
		Status string `json:"status"`
		Value  string `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	assert.Equal(t, cm.GetTip().Header.Hash().String(), response.Value)
}

func TestHandleGetTipHeader(t *testing.T) {
	app, cm := setupTestApp(t)

	resp := httpGet(t, app, "/v2/tip/header")
	requireStatus(t, resp, 200)

	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	assert.Equal(t, cm.GetTip().Height, response.Value.Height)
}

func TestHandleGetHeaderByHeight(t *testing.T) {
	app, cm := setupTestApp(t)

	if cm.GetHeight() < 100 {
		t.Skip("Not enough headers to test")
	}

	resp := httpGet(t, app, "/v2/header/height/100")
	requireStatus(t, resp, 200)

	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	assert.Equal(t, uint32(100), response.Value.Height)
}

func TestHandleGetHeaderByHeight_NotFound(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/v2/header/height/99999999")
	requireStatus(t, resp, 200)

	var response struct {
		Status string      `json:"status"`
		Value  interface{} `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	assert.Nil(t, response.Value, "Expected null value for non-existent header")
}

func TestHandleGetHeaderByHash(t *testing.T) {
	app, cm := setupTestApp(t)

	tip := cm.GetTip()
	hash := tip.Header.Hash().String()

	resp := httpGet(t, app, "/v2/header/hash/"+hash)
	requireStatus(t, resp, 200)

	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	assert.Equal(t, tip.Height, response.Value.Height)
}

func TestHandleGetHeaderByHash_InvalidHash(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/v2/header/hash/invalid")
	requireStatus(t, resp, 400)
	requireErrorResponse(t, resp.Body)
}

func TestHandleGetHeaderByHash_NotFound(t *testing.T) {
	app, _ := setupTestApp(t)

	nonExistentHash := chainhash.Hash{}
	resp := httpGet(t, app, "/v2/header/hash/"+nonExistentHash.String())
	requireStatus(t, resp, 200)

	var response struct {
		Status string      `json:"status"`
		Value  interface{} `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	assert.Nil(t, response.Value, "Expected null value for non-existent header")
}

func TestHandleGetHeaders(t *testing.T) {
	app, cm := setupTestApp(t)

	if cm.GetHeight() < 10 {
		t.Skip("Not enough headers to test")
	}

	resp := httpGet(t, app, "/v2/headers?height=0&count=10")
	requireStatus(t, resp, 200)

	var response struct {
		Status string `json:"status"`
		Value  string `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	expectedLen := 10 * 80 * 2 // 10 headers * 80 bytes * 2 hex chars
	assert.Len(t, response.Value, expectedLen)
}

func TestHandleGetHeaders_MissingParams(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/v2/headers?height=0")
	requireStatus(t, resp, 400)
	requireErrorResponse(t, resp.Body)
}

func TestHandleRobots(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/robots.txt")
	requireStatus(t, resp, 200)
	assert.Equal(t, "text/plain", resp.Headers["Content-Type"])
	assert.Equal(t, "User-agent: *\nDisallow: /\n", string(resp.Body))
}

func TestHandleOpenAPISpec(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/openapi.yaml")
	requireStatus(t, resp, 200)
	assert.Equal(t, "application/yaml", resp.Headers["Content-Type"])
	require.NotEmpty(t, resp.Body, "Expected non-empty OpenAPI spec")
	assert.True(t, strings.HasPrefix(string(resp.Body), "openapi:"), "Expected OpenAPI spec to start with 'openapi:'")
}

func TestHandleSwaggerUI(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/docs")
	requireStatus(t, resp, 200)
	assert.Equal(t, "text/html", resp.Headers["Content-Type"])

	bodyStr := string(resp.Body)
	assert.Contains(t, bodyStr, "<!DOCTYPE html>", "Expected HTML doctype")
	assert.Contains(t, bodyStr, "swagger-ui", "Expected swagger-ui reference")
	assert.Contains(t, bodyStr, "Chaintracks API Documentation", "Expected title")
	assert.Contains(t, bodyStr, "/openapi.yaml", "Expected openapi.yaml reference")
}
