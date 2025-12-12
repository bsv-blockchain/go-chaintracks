package chainmanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/stretchr/testify/require"
)

// FuzzLoadHeadersFromFile tests loadHeadersFromFile with random binary data
// to ensure it handles malformed header files gracefully without panicking.
func FuzzLoadHeadersFromFile(f *testing.F) {
	// Seed corpus with interesting binary patterns

	// Empty file
	f.Add([]byte{})

	// Single valid-looking header (80 bytes of zeros)
	validHeader := make([]byte, 80)
	f.Add(validHeader)

	// Two headers (160 bytes)
	twoHeaders := make([]byte, 160)
	f.Add(twoHeaders)

	// Not a multiple of 80 (79 bytes)
	f.Add(make([]byte, 79))

	// Not a multiple of 80 (81 bytes)
	f.Add(make([]byte, 81))

	// Not a multiple of 80 (159 bytes)
	f.Add(make([]byte, 159))

	// Large file (1000 headers = 80,000 bytes)
	largeFile := make([]byte, 80*1000)
	f.Add(largeFile)

	// Random pattern (240 bytes = 3 headers worth)
	randomPattern := []byte{
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	f.Add(randomPattern)

	// All 0xFF bytes (80 bytes)
	allOnes := make([]byte, 80)
	for i := range allOnes {
		allOnes[i] = 0xFF
	}
	f.Add(allOnes)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Create a temporary file with the fuzzed data
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "fuzz.headers")

		err := os.WriteFile(tmpFile, data, 0o600)
		require.NoError(t, err, "Failed to write temp file")

		// Should never panic
		headers, err := loadHeadersFromFile(tmpFile)

		// Validate invariants based on input
		dataLen := len(data)

		// If data length is not a multiple of 80, should return error
		if dataLen%80 != 0 {
			require.Error(t, err, "Expected error for non-80-byte-aligned data")
			require.Nil(t, headers, "Headers should be nil on error")
			require.ErrorIs(t, err, chaintracks.ErrInvalidFileSize, "Should return ErrInvalidFileSize")
			return
		}

		// If data length is a multiple of 80, it may succeed or fail depending on header validity
		expectedHeaderCount := dataLen / 80

		if err != nil {
			// Error is acceptable if header parsing fails
			require.Nil(t, headers, "Headers should be nil when error is returned")
		} else {
			// Success: validate the returned headers
			require.NotNil(t, headers, "Headers should not be nil on success")
			require.Len(t, headers, expectedHeaderCount, "Header count mismatch")

			// Verify each header is not nil
			for i, header := range headers {
				require.NotNil(t, header, "Header at index %d should not be nil", i)
			}
		}
	})
}
