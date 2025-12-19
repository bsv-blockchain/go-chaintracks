package main

import (
	"strings"
	"testing"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

func TestHandleGetNetwork(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/v2/network")
	requireStatus(t, resp, 200)

	response := requireSuccessResponse(t, resp.Body)
	assert.Equal(t, "main", response.Value)
}

func TestHandleGetTip(t *testing.T) {
	app, cm := setupTestApp(t)
	ctx := t.Context()

	resp := httpGet(t, app, "/v2/tip")
	requireStatus(t, resp, 200)
	assert.Equal(t, "no-cache", resp.Headers["Cache-Control"])

	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	assert.Equal(t, cm.GetTip(ctx).Height, response.Value.Height)
	assert.Equal(t, cm.GetTip(ctx).Hash.String(), response.Value.Hash.String())
}

func TestHandleGetHeaderByHeight(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/v2/header/height/0")
	requireStatus(t, resp, 200)

	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}
	parseJSONResponse(t, resp.Body, &response)

	assert.Equal(t, "success", response.Status)
	assert.Equal(t, uint32(0), response.Value.Height)
}

func TestHandleGetHeaderByHeight_NotFound(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/v2/header/height/99999999")
	requireStatus(t, resp, 404)
	requireErrorResponse(t, resp.Body)
}

func TestHandleGetHeaderByHash(t *testing.T) {
	app, cm := setupTestApp(t)
	ctx := t.Context()

	tip := cm.GetTip(ctx)
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
	requireStatus(t, resp, 404)
	requireErrorResponse(t, resp.Body)
}

func TestHandleGetHeaders(t *testing.T) {
	app, _ := setupTestApp(t)

	resp := httpGet(t, app, "/v2/headers?height=0&count=10")
	requireStatus(t, resp, 200)
	assert.Equal(t, "application/octet-stream", resp.Headers["Content-Type"])

	expectedLen := 10 * 80 // 10 headers * 80 bytes
	assert.Len(t, resp.Body, expectedLen)
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
