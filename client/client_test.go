package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name            string
		baseURL         string
		expectedBaseURL string
	}{
		{
			name:            "AddsHTTPPrefixWhenMissing",
			baseURL:         "example.com:3011",
			expectedBaseURL: "http://example.com:3011",
		},
		{
			name:            "PreservesHTTPPrefix",
			baseURL:         "http://example.com:3011",
			expectedBaseURL: "http://example.com:3011",
		},
		{
			name:            "PreservesHTTPSPrefix",
			baseURL:         "https://example.com:3011",
			expectedBaseURL: "https://example.com:3011",
		},
		{
			name:            "RemovesTrailingSlash",
			baseURL:         "http://example.com:3011/",
			expectedBaseURL: "http://example.com:3011",
		},
		{
			name:            "RemovesTrailingSlashWithoutProtocol",
			baseURL:         "example.com:3011/",
			expectedBaseURL: "http://example.com:3011",
		},
		{
			name:            "HandlesMultipleTrailingSlashes",
			baseURL:         "http://example.com:3011///",
			expectedBaseURL: "http://example.com:3011//",
		},
		{
			name:            "HandlesLocalhostWithHTTP",
			baseURL:         "http://localhost:3011",
			expectedBaseURL: "http://localhost:3011",
		},
		{
			name:            "HandlesLocalhostWithoutProtocol",
			baseURL:         "localhost:3011",
			expectedBaseURL: "http://localhost:3011",
		},
		{
			name:            "HandlesIPAddressWithoutProtocol",
			baseURL:         "192.168.1.1:3011",
			expectedBaseURL: "http://192.168.1.1:3011",
		},
		{
			name:            "HandlesIPAddressWithHTTP",
			baseURL:         "http://192.168.1.1:3011",
			expectedBaseURL: "http://192.168.1.1:3011",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(tt.baseURL)
			require.NotNil(t, client)
			assert.Equal(t, tt.expectedBaseURL, client.baseURL)
			assert.NotNil(t, client.httpClient)
		})
	}
}

func TestClientGetTip(t *testing.T) {
	t.Run("ReturnsNilWhenServerReturnsError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := New(server.URL)
		result := client.GetTip(t.Context())
		assert.Nil(t, result)
	})

	t.Run("ReturnsCachedTipWhenSet", func(t *testing.T) {
		hash := chainhash.Hash{1, 2, 3}
		client := &Client{
			httpClient: &http.Client{},
			currentTip: &chaintracks.BlockHeader{
				Header: &block.Header{},
				Height: 12345,
				Hash:   hash,
			},
		}
		result := client.GetTip(t.Context())
		require.NotNil(t, result)
		assert.Equal(t, uint32(12345), result.Height)
		assert.Equal(t, hash, result.Hash)
	})

	t.Run("FetchesTipFromServerWhenNotCached", func(t *testing.T) {
		expectedHash := chainhash.Hash{4, 5, 6}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := struct {
				Status string                   `json:"status"`
				Value  *chaintracks.BlockHeader `json:"value"`
			}{
				Status: "success",
				Value: &chaintracks.BlockHeader{
					Header: &block.Header{},
					Height: 999,
					Hash:   expectedHash,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := New(server.URL)
		result := client.GetTip(t.Context())
		require.NotNil(t, result)
		assert.Equal(t, uint32(999), result.Height)
		assert.Equal(t, expectedHash, result.Hash)
	})
}

func TestClientGetHeight(t *testing.T) {
	t.Run("ReturnsZeroWhenServerReturnsError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := New(server.URL)
		result := client.GetHeight(t.Context())
		assert.Equal(t, uint32(0), result)
	})

	t.Run("ReturnsCachedHeightWhenSet", func(t *testing.T) {
		client := &Client{
			httpClient: &http.Client{},
			currentTip: &chaintracks.BlockHeader{
				Header: &block.Header{},
				Height: 12345,
			},
		}
		result := client.GetHeight(t.Context())
		assert.Equal(t, uint32(12345), result)
	})

	t.Run("ReturnsHighBlockHeight", func(t *testing.T) {
		client := &Client{
			httpClient: &http.Client{},
			currentTip: &chaintracks.BlockHeader{
				Header: &block.Header{},
				Height: 800000,
			},
		}
		result := client.GetHeight(t.Context())
		assert.Equal(t, uint32(800000), result)
	})
}

func TestClientGetHeaderByHeight(t *testing.T) {
	tests := []struct {
		name          string
		height        uint32
		setupServer   func() *httptest.Server
		expectedHash  chainhash.Hash
		expectedError error
	}{
		{
			name:   "ReturnsHeaderForValidHeight",
			height: 100,
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/v2/header/height/100", r.URL.Path)
					response := map[string]interface{}{
						"status": "success",
						"value": map[string]interface{}{
							"height": 100,
							"hash":   "0101010101010101010101010101010101010101010101010101010101010101",
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			expectedHash:  chainhash.Hash{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			expectedError: nil,
		},
		{
			name:   "ReturnsErrorWhenServerReturnsNonSuccess",
			height: 200,
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					response := map[string]interface{}{
						"status": "error",
						"value":  nil,
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			expectedError: chaintracks.ErrHeaderNotFound,
		},
		{
			name:   "ReturnsErrorWhenServerReturnsNilValue",
			height: 300,
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					response := map[string]interface{}{
						"status": "success",
						"value":  nil,
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			expectedError: chaintracks.ErrHeaderNotFound,
		},
		{
			name:   "ReturnsErrorWhenServerReturnsNonOKStatus",
			height: 400,
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			expectedError: chaintracks.ErrServerRequestFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client := New(server.URL)
			result, err := client.GetHeaderByHeight(t.Context(), tt.height)

			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedHash, result.Hash)
				assert.Equal(t, tt.height, result.Height)
			}
		})
	}
}

func TestClientGetHeaderByHash(t *testing.T) {
	testHash := chainhash.Hash{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}

	tests := []struct {
		name           string
		hash           *chainhash.Hash
		setupServer    func() *httptest.Server
		expectedHeight uint32
		expectedError  error
	}{
		{
			name: "ReturnsHeaderForValidHash",
			hash: &testHash,
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/v2/header/hash/0101010101010101010101010101010101010101010101010101010101010101", r.URL.Path)
					response := map[string]interface{}{
						"status": "success",
						"value": map[string]interface{}{
							"height": 100,
							"hash":   "0101010101010101010101010101010101010101010101010101010101010101",
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			expectedHeight: 100,
			expectedError:  nil,
		},
		{
			name: "ReturnsErrorWhenServerReturnsNonSuccess",
			hash: &testHash,
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					response := map[string]interface{}{
						"status": "error",
						"value":  nil,
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			expectedError: chaintracks.ErrHeaderNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client := New(server.URL)
			result, err := client.GetHeaderByHash(t.Context(), tt.hash)

			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedHeight, result.Height)
				assert.Equal(t, *tt.hash, result.Hash)
			}
		})
	}
}

func TestClientGetNetwork(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *httptest.Server
		expectedNetwork string
		expectedError   error
	}{
		{
			name: "ReturnsNetworkForSuccessfulResponse",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/v2/network", r.URL.Path)
					response := map[string]interface{}{
						"status": "success",
						"value":  "mainnet",
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			expectedNetwork: "mainnet",
			expectedError:   nil,
		},
		{
			name: "ReturnsTestnetNetwork",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					response := map[string]interface{}{
						"status": "success",
						"value":  "testnet",
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			expectedNetwork: "testnet",
			expectedError:   nil,
		},
		{
			name: "ReturnsErrorWhenServerReturnsNonSuccess",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					response := map[string]interface{}{
						"status": "error",
						"value":  "",
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			expectedError: chaintracks.ErrServerReturnedError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client := New(server.URL)
			result, err := client.GetNetwork(t.Context())

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedNetwork, result)
			}
		})
	}
}

func TestClientCurrentHeight(t *testing.T) {
	t.Run("ReturnsZeroWhenServerReturnsError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := New(server.URL)
		result, err := client.CurrentHeight(t.Context())
		require.NoError(t, err)
		assert.Equal(t, uint32(0), result)
	})

	t.Run("ReturnsCachedHeightWhenSet", func(t *testing.T) {
		client := &Client{
			httpClient: &http.Client{},
			currentTip: &chaintracks.BlockHeader{
				Header: &block.Header{},
				Height: 54321,
			},
		}
		result, err := client.CurrentHeight(t.Context())
		require.NoError(t, err)
		assert.Equal(t, uint32(54321), result)
	})
}

func TestClientIsValidRootForHeight(t *testing.T) {
	validRoot := chainhash.Hash{0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa}
	invalidRoot := chainhash.Hash{0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb, 0xbb}

	tests := []struct {
		name          string
		setupServer   func() *httptest.Server
		root          *chainhash.Hash
		height        uint32
		expectedValid bool
		expectedError error
	}{
		{
			name: "ReturnsTrueForValidMerkleRoot",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					response := map[string]interface{}{
						"status": "success",
						"value": map[string]interface{}{
							"height":     100,
							"hash":       "0101010101010101010101010101010101010101010101010101010101010101",
							"merkleRoot": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			root:          &validRoot,
			height:        100,
			expectedValid: true,
			expectedError: nil,
		},
		{
			name: "ReturnsFalseForInvalidMerkleRoot",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					response := map[string]interface{}{
						"status": "success",
						"value": map[string]interface{}{
							"height":     100,
							"hash":       "0101010101010101010101010101010101010101010101010101010101010101",
							"merkleRoot": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			root:          &invalidRoot,
			height:        100,
			expectedValid: false,
			expectedError: nil,
		},
		{
			name: "ReturnsErrorWhenHeaderNotFound",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					response := map[string]interface{}{
						"status": "error",
						"value":  nil,
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			root:          &validRoot,
			height:        100,
			expectedValid: false,
			expectedError: chaintracks.ErrHeaderNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client := New(server.URL)
			valid, err := client.IsValidRootForHeight(t.Context(), tt.root, tt.height)

			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
				assert.False(t, valid)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedValid, valid)
			}
		})
	}
}
