package main

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bsv-blockchain/go-chaintracks/chainmanager"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
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

// setupTestApp creates a test Fiber app with all routes configured.
func setupTestApp(t *testing.T) (*fiber.App, *chainmanager.ChainManager) {
	t.Helper()

	ctx := t.Context()

	// Create temp directory and copy test data files
	tempDir := t.TempDir()
	copyTestData(t, "testdata", tempDir)

	cm, err := chainmanager.NewForTesting(ctx, "main", tempDir)
	require.NoError(t, err, "Failed to create chain manager")

	server := NewServer(ctx, cm)
	app := fiber.New()
	dashboard := NewDashboardHandler(server)
	server.SetupRoutes(app, dashboard)
	return app, cm
}

// copyTestData copies all files from srcDir to dstDir.
func copyTestData(t *testing.T, srcDir, dstDir string) {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(srcDir, "*"))
	require.NoError(t, err, "Failed to glob testdata")

	for _, srcFile := range files {
		data, err := os.ReadFile(srcFile) //nolint:gosec // Test helper reading from known testdata directory
		require.NoError(t, err, "Failed to read testdata file")

		dstFile := filepath.Join(dstDir, filepath.Base(srcFile))
		err = os.WriteFile(dstFile, data, 0o600)
		require.NoError(t, err, "Failed to write testdata file")
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
