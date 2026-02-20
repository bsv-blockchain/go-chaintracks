package chainmanager

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

// Valid 80-byte header in hex (Bitcoin genesis block header)
const validHeaderHex = "0100000000000000000000000000000000000000000000000000000000000000000000003ba3edfd7a7b12b27ac72c3e67768f617fc81bc3888a51323a9fb8aa4b1e5e4a29ab5f49ffff001d1dac2b7c"

func getValidHeaderBytes(t *testing.T) []byte {
	t.Helper()
	headerBytes, err := hex.DecodeString(validHeaderHex)
	require.NoError(t, err)
	require.Len(t, headerBytes, 80)
	return headerBytes
}

func TestNewCDNBootstrapper(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		network string
	}{
		{
			name:    "CreatesBootstrapperWithMainnet",
			baseURL: "https://example.com",
			network: "main",
		},
		{
			name:    "CreatesBootstrapperWithTestnet",
			baseURL: "https://test.example.com",
			network: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewCDNBootstrapper(tt.baseURL, tt.network)

			require.NotNil(t, b)
			assert.Equal(t, tt.baseURL, b.baseURL)
			assert.Equal(t, tt.network, b.network)
			assert.NotNil(t, b.httpClient)
		})
	}
}

func TestCDNBootstrapperFetchMetadata(t *testing.T) {
	validMetadata := chaintracks.CDNMetadata{
		RootFolder:     "",
		JSONFilename:   "mainNetBlockHeaders.json",
		HeadersPerFile: 100000,
		Files: []chaintracks.CDNFileEntry{
			{
				Chain:       "main",
				Count:       100000,
				FileHash:    "abc123",
				FileName:    "mainNet_0.headers",
				FirstHeight: 0,
			},
		},
	}

	tests := []struct {
		name          string
		network       string
		setupServer   func() *httptest.Server
		expectedMeta  *chaintracks.CDNMetadata
		expectedError bool
	}{
		{
			name:    "FetchesMetadataSuccessfully",
			network: "main",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/mainNetBlockHeaders.json", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(validMetadata)
				}))
			},
			expectedMeta:  &validMetadata,
			expectedError: false,
		},
		{
			name:    "FetchesMetadataForTestnet",
			network: "test",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/testNetBlockHeaders.json", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(validMetadata)
				}))
			},
			expectedMeta:  &validMetadata,
			expectedError: false,
		},
		{
			name:    "ReturnsErrorOnNotFound",
			network: "main",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			expectedMeta:  nil,
			expectedError: true,
		},
		{
			name:    "ReturnsErrorOnInvalidJSON",
			network: "main",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte("invalid json"))
				}))
			},
			expectedMeta:  nil,
			expectedError: true,
		},
		{
			name:    "ReturnsErrorOnServerError",
			network: "main",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			expectedMeta:  nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			b := NewCDNBootstrapper(server.URL, tt.network)
			metadata, err := b.FetchMetadata(context.Background())

			if tt.expectedError {
				require.Error(t, err)
				assert.Nil(t, metadata)
			} else {
				require.NoError(t, err)
				require.NotNil(t, metadata)
				assert.Equal(t, tt.expectedMeta.HeadersPerFile, metadata.HeadersPerFile)
				assert.Len(t, metadata.Files, len(tt.expectedMeta.Files))
			}
		})
	}
}

func TestCDNBootstrapperFetchHeadersFile(t *testing.T) {
	validHeader := getValidHeaderBytes(t)

	// Create a buffer with 3 headers
	threeHeaders := make([]byte, 0, 240)
	for i := 0; i < 3; i++ {
		threeHeaders = append(threeHeaders, validHeader...)
	}

	tests := []struct {
		name          string
		fileName      string
		setupServer   func() *httptest.Server
		expectedLen   int
		expectedError bool
	}{
		{
			name:     "FetchesSingleHeader",
			fileName: "mainNet_0.headers",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/mainNet_0.headers", r.URL.Path)
					_, _ = w.Write(validHeader)
				}))
			},
			expectedLen:   80,
			expectedError: false,
		},
		{
			name:     "FetchesMultipleHeaders",
			fileName: "mainNet_0.headers",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write(threeHeaders)
				}))
			},
			expectedLen:   240,
			expectedError: false,
		},
		{
			name:     "ReturnsErrorOnNotFound",
			fileName: "mainNet_999.headers",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			expectedLen:   0,
			expectedError: true,
		},
		{
			name:     "ReturnsErrorOnInvalidSize",
			fileName: "mainNet_0.headers",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					// Return 50 bytes - not a multiple of 80
					_, _ = w.Write(make([]byte, 50))
				}))
			},
			expectedLen:   0,
			expectedError: true,
		},
		{
			name:     "ReturnsErrorOnServerError",
			fileName: "mainNet_0.headers",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			expectedLen:   0,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			b := NewCDNBootstrapper(server.URL, "main")
			data, err := b.FetchHeadersFile(context.Background(), tt.fileName)

			if tt.expectedError {
				require.Error(t, err)
				assert.Nil(t, data)
			} else {
				require.NoError(t, err)
				assert.Len(t, data, tt.expectedLen)
			}
		})
	}
}

func TestParseHeadersFromBytes(t *testing.T) {
	validHeader := getValidHeaderBytes(t)

	tests := []struct {
		name          string
		data          []byte
		expectedCount int
		expectedError bool
	}{
		{
			name:          "ParsesSingleHeader",
			data:          validHeader,
			expectedCount: 1,
			expectedError: false,
		},
		{
			name: "ParsesMultipleHeaders",
			data: func() []byte {
				data := make([]byte, 0, 240)
				for i := 0; i < 3; i++ {
					data = append(data, validHeader...)
				}
				return data
			}(),
			expectedCount: 3,
			expectedError: false,
		},
		{
			name:          "ParsesEmptyData",
			data:          []byte{},
			expectedCount: 0,
			expectedError: false,
		},
		{
			name:          "ParsesZeroHeader",
			data:          make([]byte, 80), // All zeros - still parses (sdk doesn't validate content)
			expectedCount: 1,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers, err := parseHeadersFromBytes(tt.data)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, headers, tt.expectedCount)
			}
		})
	}
}

func TestCDNBootstrapperConvertToBlockHeaders(t *testing.T) {
	validHeader := getValidHeaderBytes(t)

	// Parse a valid header to use in tests
	headers, err := parseHeadersFromBytes(validHeader)
	require.NoError(t, err)
	require.Len(t, headers, 1)

	tests := []struct {
		name          string
		headers       int
		firstHeight   uint32
		setupCM       func() *ChainManager
		expectedError bool
	}{
		{
			name:        "ConvertsGenesisBlock",
			headers:     1,
			firstHeight: 0,
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: make([]chainhash.Hash, 0),
					byHash:   make(map[chainhash.Hash]*chaintracks.BlockHeader),
				}
			},
			expectedError: false,
		},
		{
			name:        "ConvertsMultipleBlocks",
			headers:     3,
			firstHeight: 0,
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: make([]chainhash.Hash, 0),
					byHash:   make(map[chainhash.Hash]*chaintracks.BlockHeader),
				}
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the required number of headers
			inputHeaders := make([]*block.Header, 0, tt.headers)
			for i := 0; i < tt.headers; i++ {
				inputHeaders = append(inputHeaders, headers[0])
			}

			cm := tt.setupCM()
			b := NewCDNBootstrapper("https://example.com", "main")

			blockHeaders, err := b.convertToBlockHeaders(context.Background(), cm, inputHeaders, tt.firstHeight)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Len(t, blockHeaders, tt.headers)

				// Verify heights are set correctly
				for i, bh := range blockHeaders {
					assert.Equal(t, tt.firstHeight+uint32(i), bh.Height)
					assert.NotNil(t, bh.ChainWork)
				}
			}
		})
	}
}

func TestCDNBootstrapperBootstrapIntegration(t *testing.T) {
	validHeader := getValidHeaderBytes(t)

	// Create metadata with one file
	metadata := chaintracks.CDNMetadata{
		RootFolder:     "",
		JSONFilename:   "mainNetBlockHeaders.json",
		HeadersPerFile: 100000,
		Files: []chaintracks.CDNFileEntry{
			{
				Chain:       "main",
				Count:       1,
				FileHash:    "abc123",
				FileName:    "mainNet_0.headers",
				FirstHeight: 0,
			},
		},
	}

	tests := []struct {
		name          string
		setupServer   func() *httptest.Server
		expectedError bool
	}{
		{
			name: "BootstrapsSuccessfully",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/mainNetBlockHeaders.json":
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(metadata)
					case "/mainNet_0.headers":
						_, _ = w.Write(validHeader)
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
			},
			expectedError: false,
		},
		{
			name: "ReturnsErrorOnMetadataFailure",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			expectedError: true,
		},
		{
			name: "ReturnsErrorOnHeaderFileFailure",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/mainNetBlockHeaders.json":
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(metadata)
					case "/mainNet_0.headers":
						w.WriteHeader(http.StatusNotFound)
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			// Create a minimal chain manager for testing
			cm := &ChainManager{
				byHeight:         make([]chainhash.Hash, 0),
				byHash:           make(map[chainhash.Hash]*chaintracks.BlockHeader),
				network:          "main",
				localStoragePath: t.TempDir(),
			}

			b := NewCDNBootstrapper(server.URL, "main")
			err := b.Bootstrap(context.Background(), cm)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify chain tip was set
				tip := cm.GetTip(context.Background())
				assert.NotNil(t, tip)
				assert.Equal(t, uint32(0), tip.Height)
			}
		})
	}
}

func TestCDNBootstrapperContextCancellation(t *testing.T) {
	validHeader := getValidHeaderBytes(t)

	metadata := chaintracks.CDNMetadata{
		RootFolder:     "",
		JSONFilename:   "mainNetBlockHeaders.json",
		HeadersPerFile: 100000,
		Files: []chaintracks.CDNFileEntry{
			{
				Chain:       "main",
				Count:       1,
				FileHash:    "abc123",
				FileName:    "mainNet_0.headers",
				FirstHeight: 0,
			},
			{
				Chain:       "main",
				Count:       1,
				FileHash:    "def456",
				FileName:    "mainNet_1.headers",
				FirstHeight: 100000,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mainNetBlockHeaders.json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(metadata)
		case "/mainNet_0.headers":
			_, _ = w.Write(validHeader)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cm := &ChainManager{
		byHeight:         make([]chainhash.Hash, 0),
		byHash:           make(map[chainhash.Hash]*chaintracks.BlockHeader),
		network:          "main",
		localStoragePath: t.TempDir(),
	}

	// Cancel context before bootstrap
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	b := NewCDNBootstrapper(server.URL, "main")
	err := b.Bootstrap(ctx, cm)

	assert.Error(t, err)
}

func TestCDNConstants(t *testing.T) {
	// Verify constants are set correctly
	assert.Equal(t, 100000, cdnHeadersPerFile)
	assert.Equal(t, 80, cdnHeaderSize)
}
