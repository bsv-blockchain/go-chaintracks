package chaintracks

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchLatestBlock(t *testing.T) {
	// Valid 80-byte header in hex (this is a sample - in real use it would be a valid Bitcoin header)
	validHeaderHex := "0100000000000000000000000000000000000000000000000000000000000000000000003ba3edfd7a7b12b27ac72c3e67768f617fc81bc3888a51323a9fb8aa4b1e5e4a29ab5f49ffff001d1dac2b7c"
	validHeaderBytes, err := hex.DecodeString(validHeaderHex)
	require.NoError(t, err)
	require.Len(t, validHeaderBytes, 80, "Valid header should be 80 bytes")

	tests := []struct {
		name          string
		setupServer   func() *httptest.Server
		expectedError error
	}{
		{
			name: "ReturnsHashForValidResponse",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/bestblockheader", r.URL.Path)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(validHeaderBytes)
				}))
			},
			expectedError: nil,
		},
		{
			name: "ReturnsErrorWhenServerReturnsNonOKStatus",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			expectedError: ErrBestBlockHeaderFailed,
		},
		{
			name: "ReturnsErrorWhenResponseSizeIsInvalid",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					// Return 50 bytes instead of 80
					_, _ = w.Write(make([]byte, 50))
				}))
			},
			expectedError: ErrInvalidHeaderSize,
		},
		{
			name: "ReturnsErrorWhenResponseIsEmpty",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					// Return empty response
				}))
			},
			expectedError: ErrInvalidHeaderSize,
		},
		{
			name: "ReturnsErrorWhenResponseIsTooLarge",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					// Return 100 bytes instead of 80
					_, _ = w.Write(make([]byte, 100))
				}))
			},
			expectedError: ErrInvalidHeaderSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			hash, err := FetchLatestBlock(t.Context(), server.URL)

			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
				// For valid response, just check that we got a non-zero hash
				assert.NotEqual(t, chainhash.Hash{}, hash)
			}
		})
	}
}
