package chaintracks

import (
	"os"
	"path/filepath"
	"testing"
)

const testCDNPath = "../../../chaintracks-server/public/headers"

func TestParseMetadata(t *testing.T) {
	metadataPath := filepath.Join(testCDNPath, "mainNetBlockHeaders.json")

	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Skipf("Test CDN data not found at %s", metadataPath)
	}

	metadata, err := parseMetadata(metadataPath)
	if err != nil {
		t.Fatalf("Failed to parse metadata: %v", err)
	}

	if metadata.HeadersPerFile != 100000 {
		t.Errorf("Expected 100000 headers per file, got %d", metadata.HeadersPerFile)
	}

	if len(metadata.Files) == 0 {
		t.Error("Expected at least one file entry")
	}

	t.Logf("Parsed metadata with %d files", len(metadata.Files))
	for i, file := range metadata.Files {
		//nolint:gosec // Test code: Count is from test data
		t.Logf("  File %d: %s, height %d-%d, count %d",
			i, file.FileName, file.FirstHeight, file.FirstHeight+uint32(file.Count)-1, file.Count)
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

	cm, err := NewChainManager("main", testCDNPath)
	if err != nil {
		t.Fatalf("Failed to create ChainManager: %v", err)
	}

	if cm.GetHeight() == 0 {
		t.Skip("No local files found, this is expected")
	}

	t.Logf("Loaded chain to height %d", cm.GetHeight())

	tip := cm.GetTip()
	if tip == nil {
		t.Fatal("Chain tip is nil")
	}

	t.Logf("Chain tip: height=%d, hash=%s", tip.Height, tip.Header.Hash().String())
}
