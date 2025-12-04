package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	p2p "github.com/bsv-blockchain/go-p2p-message-bus"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-chaintracks/pkg/chaintracks"
)

// getConfigEnvVars returns the environment variables used for configuration
func getConfigEnvVars() []string {
	return []string{"PORT", "CHAIN", "STORAGE_PATH", "BOOTSTRAP_URL"}
}

// withEnvVars sets environment variables for a test and returns a cleanup function
// that restores the original values. Pass nil values to unset variables.
func withEnvVars(t *testing.T, vars map[string]string) func() {
	t.Helper()

	configEnvVars := getConfigEnvVars()

	// Backup current values
	backup := make(map[string]string)
	exists := make(map[string]bool)
	for _, key := range configEnvVars {
		if val, ok := os.LookupEnv(key); ok {
			backup[key] = val
			exists[key] = true
		}
	}

	// Clear all config vars first
	for _, key := range configEnvVars {
		_ = os.Unsetenv(key)
	}

	// Set requested values
	for key, value := range vars {
		_ = os.Setenv(key, value)
	}

	// Return cleanup function
	return func() {
		for _, key := range configEnvVars {
			if exists[key] {
				_ = os.Setenv(key, backup[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}
}

// setupTestApp creates a test Fiber app with all routes configured
func setupTestApp(t *testing.T) (*fiber.App, *chaintracks.ChainManager) {
	t.Helper()

	ctx := context.Background()

	// Create temp directory and copy checkpoint files
	tempDir := t.TempDir()
	copyCheckpointFiles(t, "../../data/headers", tempDir, "main")

	privKey, err := chaintracks.LoadOrGeneratePrivateKey(tempDir)
	require.NoError(t, err, "Failed to load or generate private key")

	p2pClient, err := p2p.NewClient(p2p.Config{
		Name:          "go-chaintracks-test",
		Logger:        &p2p.DefaultLogger{},
		PrivateKey:    privKey,
		Port:          0,
		PeerCacheFile: filepath.Join(tempDir, "peer_cache.json"),
	})
	require.NoError(t, err, "Failed to create P2P client")

	cm, err := chaintracks.NewChainManager(ctx, "main", tempDir, p2pClient, "")
	require.NoError(t, err, "Failed to create chain manager")

	server := NewServer(cm)
	app := fiber.New()
	dashboard := NewDashboardHandler(server)
	server.SetupRoutes(app, dashboard)
	return app, cm
}

// copyCheckpointFiles copies checkpoint header files to a temp directory
func copyCheckpointFiles(t *testing.T, srcDir, dstDir, network string) {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(srcDir, network+"*"))
	if err != nil || len(files) == 0 {
		return
	}

	for _, srcFile := range files {
		data, err := os.ReadFile(srcFile) //nolint:gosec // Test helper reading from known checkpoint directory
		require.NoError(t, err, "Failed to read checkpoint file")

		dstFile := filepath.Join(dstDir, filepath.Base(srcFile))
		err = os.WriteFile(dstFile, data, 0o600)
		require.NoError(t, err, "Failed to write checkpoint file")
	}
}

// testResponse holds the result of an HTTP test request
type testResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// httpGet performs a GET request and returns the response data
func httpGet(t *testing.T, app *fiber.App, path string) testResponse {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	resp, err := app.Test(req)
	require.NoError(t, err, "Failed to make request")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response body")
	_ = resp.Body.Close()

	headers := make(map[string]string)
	for key := range resp.Header {
		headers[key] = resp.Header.Get(key)
	}

	return testResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
	}
}

// requireStatus checks that the response has the expected status code
func requireStatus(t *testing.T, resp testResponse, expected int) {
	t.Helper()
	require.Equal(t, expected, resp.StatusCode, "Unexpected status code")
}

// parseJSONResponse unmarshals JSON response body into the provided pointer
func parseJSONResponse(t *testing.T, body []byte, v interface{}) {
	t.Helper()
	err := json.Unmarshal(body, v)
	require.NoError(t, err, "Failed to decode JSON response")
}

// requireSuccessResponse checks for status "success" in a Response
func requireSuccessResponse(t *testing.T, body []byte) Response {
	t.Helper()
	var response Response
	parseJSONResponse(t, body, &response)
	require.Equal(t, "success", response.Status, "Expected success status")
	return response
}

// requireErrorResponse checks for status "error" in a Response
func requireErrorResponse(t *testing.T, body []byte) {
	t.Helper()
	var response Response
	parseJSONResponse(t, body, &response)
	require.Equal(t, "error", response.Status, "Expected error status")
}
