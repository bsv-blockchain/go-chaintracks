package chaintracks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	p2p "github.com/bsv-blockchain/go-p2p-message-bus"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCDNPath = "../../../chaintracks-server/public/headers"

func TestParseMetadata(t *testing.T) {
	tests := []struct {
		name        string
		setupFile   func(t *testing.T) string
		expectError bool
		errContains string
		validate    func(t *testing.T, metadata *CDNMetadata)
	}{
		{
			name: "ValidMetadataFile",
			setupFile: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "test.json")

				hash := chainhash.Hash{}
				metadata := CDNMetadata{
					RootFolder:     "headers",
					JSONFilename:   "mainNetBlockHeaders.json",
					HeadersPerFile: 10000,
					Files: []CDNFileEntry{
						{
							Chain:         "main",
							Count:         100,
							FileHash:      "abc123",
							FileName:      "00000000-00000099.headers",
							FirstHeight:   0,
							LastChainWork: "0000000000000000000000000000000000000000000000000000000000001234",
							LastHash:      hash,
						},
					},
				}

				data, err := json.MarshalIndent(metadata, "", "  ")
				require.NoError(t, err)
				err = os.WriteFile(filePath, data, 0o600)
				require.NoError(t, err)

				return filePath
			},
			expectError: false,
			validate: func(t *testing.T, metadata *CDNMetadata) {
				t.Helper()
				require.NotNil(t, metadata)
				assert.Equal(t, "headers", metadata.RootFolder)
				assert.Equal(t, "mainNetBlockHeaders.json", metadata.JSONFilename)
				assert.Equal(t, 10000, metadata.HeadersPerFile)
				assert.Len(t, metadata.Files, 1)
				assert.Equal(t, "main", metadata.Files[0].Chain)
				assert.Equal(t, 100, metadata.Files[0].Count)
				assert.Equal(t, uint32(0), metadata.Files[0].FirstHeight)
			},
		},
		{
			name: "FileDoesNotExist",
			setupFile: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nonexistent.json")
			},
			expectError: true,
			errContains: "failed to read metadata",
			validate:    nil,
		},
		{
			name: "InvalidJSONFormat",
			setupFile: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "invalid.json")

				err := os.WriteFile(filePath, []byte("{ invalid json }"), 0o600)
				require.NoError(t, err)

				return filePath
			},
			expectError: true,
			errContains: "failed to parse metadata JSON",
			validate:    nil,
		},
		{
			name: "EmptyFile",
			setupFile: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "empty.json")

				err := os.WriteFile(filePath, []byte(""), 0o600)
				require.NoError(t, err)

				return filePath
			},
			expectError: true,
			errContains: "failed to parse metadata JSON",
			validate:    nil,
		},
		{
			name: "MinimalValidJSON",
			setupFile: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "minimal.json")

				metadata := CDNMetadata{
					Files: []CDNFileEntry{},
				}

				data, err := json.Marshal(metadata)
				require.NoError(t, err)
				err = os.WriteFile(filePath, data, 0o600)
				require.NoError(t, err)

				return filePath
			},
			expectError: false,
			validate: func(t *testing.T, metadata *CDNMetadata) {
				t.Helper()
				require.NotNil(t, metadata)
				assert.Empty(t, metadata.Files)
				assert.Empty(t, metadata.RootFolder)
				assert.Equal(t, 0, metadata.HeadersPerFile)
			},
		},
		{
			name: "IntegrationTestWithRealCDNData",
			setupFile: func(t *testing.T) string {
				t.Helper()
				metadataPath := filepath.Join(testCDNPath, "mainNetBlockHeaders.json")
				if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
					t.Skipf("Test CDN data not found at %s", metadataPath)
				}
				return metadataPath
			},
			expectError: false,
			validate: func(t *testing.T, metadata *CDNMetadata) {
				t.Helper()
				require.NotNil(t, metadata)
				assert.Equal(t, 100000, metadata.HeadersPerFile, "Expected 100000 headers per file")
				assert.NotEmpty(t, metadata.Files, "Expected at least one file entry")
				t.Logf("Parsed metadata with %d files", len(metadata.Files))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFile(t)

			metadata, err := parseMetadata(filePath)

			if tt.expectError {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, metadata)
			} else {
				require.NoError(t, err)
				require.NotNil(t, metadata)
				if tt.validate != nil {
					tt.validate(t, metadata)
				}
			}
		})
	}
}

func TestLoadHeadersFromFile(t *testing.T) {
	filePath := filepath.Join(testCDNPath, "mainNet_0.headers")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Skipf("Test file not found at %s", filePath)
	}

	headers, err := loadHeadersFromFile(filePath)
	if err != nil {
		t.Fatalf("Failed to load headers: %v", err)
	}

	if len(headers) != 100000 {
		t.Errorf("Expected 100000 headers, got %d", len(headers))
	}

	genesisHash := headers[0].Hash()
	expectedGenesisHash := "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"
	if genesisHash.String() != expectedGenesisHash {
		t.Errorf("Genesis hash mismatch:\n  got:      %s\n  expected: %s",
			genesisHash.String(), expectedGenesisHash)
	}

	t.Logf("Loaded %d headers successfully", len(headers))
	t.Logf("Genesis hash: %s", genesisHash.String())
}

func TestLoadFromLocalFiles(t *testing.T) {
	if _, err := os.Stat(testCDNPath); os.IsNotExist(err) {
		t.Skipf("Test CDN data not found at %s", testCDNPath)
	}

	ctx := t.Context()

	// Load or generate private key for P2P
	privKey, err := LoadOrGeneratePrivateKey(testCDNPath)
	if err != nil {
		t.Fatalf("Failed to load or generate private key: %v", err)
	}

	// Create P2P client
	p2pClient, err := p2p.NewClient(p2p.Config{
		Name:          "go-chaintracks-test",
		Logger:        &p2p.DefaultLogger{},
		PrivateKey:    privKey,
		Port:          0,
		PeerCacheFile: filepath.Join(testCDNPath, "peer_cache.json"),
	})
	if err != nil {
		t.Fatalf("Failed to create P2P client: %v", err)
	}

	cm, err := NewChainManager(ctx, "main", testCDNPath, p2pClient)
	if err != nil {
		t.Fatalf("Failed to create ChainManager: %v", err)
	}

	if cm.GetHeight(ctx) == 0 {
		t.Skip("No local files found, this is expected")
	}

	t.Logf("Loaded chain to height %d", cm.GetHeight(ctx))

	tip := cm.GetTip(ctx)
	if tip == nil {
		t.Fatal("Chain tip is nil")
	}

	t.Logf("Chain tip: height=%d, hash=%s", tip.Height, tip.Header.Hash().String())
}

func TestChainManagerWriteLocalMetadata(t *testing.T) {
	tests := []struct {
		name        string
		setupCM     func(t *testing.T) *ChainManager
		metadata    *CDNMetadata
		expectError bool
		errContains string
		validate    func(t *testing.T, cm *ChainManager)
	}{
		{
			name: "SuccessfulWriteWithValidMetadata",
			setupCM: func(t *testing.T) *ChainManager {
				t.Helper()
				tmpDir := t.TempDir()
				return &ChainManager{
					localStoragePath: tmpDir,
					network:          "main",
				}
			},
			metadata: &CDNMetadata{
				RootFolder:     "headers",
				JSONFilename:   "mainNetBlockHeaders.json",
				HeadersPerFile: 10000,
				Files: []CDNFileEntry{
					{
						Chain:         "main",
						Count:         100,
						FileHash:      "abc123",
						FileName:      "00000000-00000099.headers",
						FirstHeight:   0,
						LastChainWork: "0000000000000000000000000000000000000000000000000000000000001234",
						LastHash:      chainhash.Hash{},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, cm *ChainManager) {
				t.Helper()
				// Verify file was created
				metadataPath := filepath.Join(cm.localStoragePath, cm.network+"NetBlockHeaders.json")
				_, err := os.Stat(metadataPath)
				require.NoError(t, err, "metadata file should exist")

				// Verify content
				data, err := os.ReadFile(metadataPath) //nolint:gosec // Test code: path is from t.TempDir()
				require.NoError(t, err)

				var readMetadata CDNMetadata
				err = json.Unmarshal(data, &readMetadata)
				require.NoError(t, err)

				assert.Equal(t, "headers", readMetadata.RootFolder)
				assert.Equal(t, "mainNetBlockHeaders.json", readMetadata.JSONFilename)
				assert.Equal(t, 10000, readMetadata.HeadersPerFile)
				assert.Len(t, readMetadata.Files, 1)
			},
		},
		{
			name: "EmptyStoragePathSkipsWrite",
			setupCM: func(t *testing.T) *ChainManager {
				t.Helper()
				return &ChainManager{
					localStoragePath: "",
					network:          "main",
				}
			},
			metadata: &CDNMetadata{
				RootFolder: "headers",
				Files:      []CDNFileEntry{},
			},
			expectError: false,
			validate: func(t *testing.T, _ *ChainManager) {
				t.Helper()
				// Nothing to validate - no file should be created
			},
		},
		{
			name: "MinimalMetadata",
			setupCM: func(t *testing.T) *ChainManager {
				t.Helper()
				tmpDir := t.TempDir()
				return &ChainManager{
					localStoragePath: tmpDir,
					network:          "test",
				}
			},
			metadata: &CDNMetadata{
				Files: []CDNFileEntry{},
			},
			expectError: false,
			validate: func(t *testing.T, cm *ChainManager) {
				t.Helper()
				metadataPath := filepath.Join(cm.localStoragePath, cm.network+"NetBlockHeaders.json")
				data, err := os.ReadFile(metadataPath) //nolint:gosec // Test code: path is from t.TempDir()
				require.NoError(t, err)

				var readMetadata CDNMetadata
				err = json.Unmarshal(data, &readMetadata)
				require.NoError(t, err)

				assert.Empty(t, readMetadata.Files)
			},
		},
		{
			name: "FilePermissionsAreCorrect",
			setupCM: func(t *testing.T) *ChainManager {
				t.Helper()
				tmpDir := t.TempDir()
				return &ChainManager{
					localStoragePath: tmpDir,
					network:          "main",
				}
			},
			metadata: &CDNMetadata{
				Files: []CDNFileEntry{},
			},
			expectError: false,
			validate: func(t *testing.T, cm *ChainManager) {
				t.Helper()
				metadataPath := filepath.Join(cm.localStoragePath, cm.network+"NetBlockHeaders.json")
				info, err := os.Stat(metadataPath)
				require.NoError(t, err)

				// Check that permissions are 0600 (owner read/write only)
				mode := info.Mode().Perm()
				assert.Equal(t, os.FileMode(0o600), mode, "file should have 0600 permissions")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM(t)

			err := cm.writeLocalMetadata(tt.metadata)

			if tt.expectError {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, cm)
				}
			}
		})
	}
}
