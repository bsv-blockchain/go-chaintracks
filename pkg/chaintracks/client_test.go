package chaintracks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
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
			client := NewClient(tt.baseURL)
			require.NotNil(t, client)
			assert.Equal(t, tt.expectedBaseURL, client.baseURL)
			assert.NotNil(t, client.httpClient)
		})
	}
}

func TestClientGetTip(t *testing.T) {
	tests := []struct {
		name        string
		setupClient func() *Client
		expected    *BlockHeader
	}{
		{
			name: "ReturnsNilWhenCurrentTipIsNil",
			setupClient: func() *Client {
				return &Client{
					currentTip: nil,
				}
			},
			expected: nil,
		},
		{
			name: "ReturnsCurrentTipWhenSet",
			setupClient: func() *Client {
				hash := chainhash.Hash{1, 2, 3}
				return &Client{
					currentTip: &BlockHeader{
						Header: &block.Header{},
						Height: 12345,
						Hash:   hash,
					},
				}
			},
			expected: &BlockHeader{
				Header: &block.Header{},
				Height: 12345,
				Hash:   chainhash.Hash{1, 2, 3},
			},
		},
		{
			name: "ReturnsGenesisBlock",
			setupClient: func() *Client {
				hash := chainhash.Hash{}
				return &Client{
					currentTip: &BlockHeader{
						Header: &block.Header{},
						Height: 0,
						Hash:   hash,
					},
				}
			},
			expected: &BlockHeader{
				Header: &block.Header{},
				Height: 0,
				Hash:   chainhash.Hash{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			ctx := context.Background()
			result := client.GetTip(ctx)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Height, result.Height)
				assert.Equal(t, tt.expected.Hash, result.Hash)
			}
		})
	}
}

func TestClientGetHeight(t *testing.T) {
	tests := []struct {
		name           string
		setupClient    func() *Client
		expectedHeight uint32
	}{
		{
			name: "ReturnsZeroWhenCurrentTipIsNil",
			setupClient: func() *Client {
				return &Client{
					currentTip: nil,
				}
			},
			expectedHeight: 0,
		},
		{
			name: "ReturnsCorrectHeightWhenCurrentTipIsSet",
			setupClient: func() *Client {
				return &Client{
					currentTip: &BlockHeader{
						Header: &block.Header{},
						Height: 12345,
					},
				}
			},
			expectedHeight: 12345,
		},
		{
			name: "ReturnsZeroForGenesisBlock",
			setupClient: func() *Client {
				return &Client{
					currentTip: &BlockHeader{
						Header: &block.Header{},
						Height: 0,
					},
				}
			},
			expectedHeight: 0,
		},
		{
			name: "ReturnsHighBlockHeight",
			setupClient: func() *Client {
				return &Client{
					currentTip: &BlockHeader{
						Header: &block.Header{},
						Height: 800000,
					},
				}
			},
			expectedHeight: 800000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			ctx := context.Background()
			result := client.GetHeight(ctx)
			assert.Equal(t, tt.expectedHeight, result)
		})
	}
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
			expectedError: ErrHeaderNotFound,
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
			expectedError: ErrHeaderNotFound,
		},
		{
			name:   "ReturnsErrorWhenServerReturnsNonOKStatus",
			height: 400,
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			expectedError: ErrServerRequestFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()
			result, err := client.GetHeaderByHeight(ctx, tt.height)

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
			expectedError: ErrHeaderNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()
			result, err := client.GetHeaderByHash(ctx, tt.hash)

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
			expectedError: ErrServerReturnedError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()
			result, err := client.GetNetwork(ctx)

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
	tests := []struct {
		name           string
		setupClient    func() *Client
		expectedHeight uint32
	}{
		{
			name: "ReturnsZeroWhenCurrentTipIsNil",
			setupClient: func() *Client {
				return &Client{
					currentTip: nil,
				}
			},
			expectedHeight: 0,
		},
		{
			name: "ReturnsCorrectHeightWhenCurrentTipIsSet",
			setupClient: func() *Client {
				return &Client{
					currentTip: &BlockHeader{
						Header: &block.Header{},
						Height: 54321,
					},
				}
			},
			expectedHeight: 54321,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			ctx := context.Background()
			result, err := client.CurrentHeight(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedHeight, result)
		})
	}
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
			expectedError: ErrHeaderNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()
			valid, err := client.IsValidRootForHeight(ctx, tt.root, tt.height)

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
