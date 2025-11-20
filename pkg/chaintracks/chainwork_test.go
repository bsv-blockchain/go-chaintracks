package chaintracks

import (
	"math/big"
	"testing"
)

func TestCompactToBig(t *testing.T) {
	tests := []struct {
		name     string
		compact  uint32
		expected string // hex representation
	}{
		{
			name:     "genesis block mainnet",
			compact:  0x1d00ffff,
			expected: "00000000ffff0000000000000000000000000000000000000000000000000000",
		},
		{
			name:     "typical difficulty",
			compact:  0x1b0404cb,
			expected: "00000000000404cb000000000000000000000000000000000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompactToBig(tt.compact)
			expected := new(big.Int)
			expected.SetString(tt.expected, 16)

			if result.Cmp(expected) != 0 {
				t.Errorf("CompactToBig(%x) = %x, expected %x", tt.compact, result, expected)
			}
		})
	}
}

func TestCalculateWork(t *testing.T) {
	tests := []struct {
		name    string
		bits    uint32
		wantPos bool // should return positive work
	}{
		{
			name:    "genesis difficulty",
			bits:    0x1d00ffff,
			wantPos: true,
		},
		{
			name:    "typical difficulty",
			bits:    0x1b0404cb,
			wantPos: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			work := CalculateWork(tt.bits)
			if tt.wantPos && work.Sign() <= 0 {
				t.Errorf("CalculateWork(%x) returned non-positive work: %v", tt.bits, work)
			}
		})
	}
}

func TestAddWork(t *testing.T) {
	bits := uint32(0x1d00ffff)
	initial := big.NewInt(0)

	result := AddWork(initial, bits)

	if result.Sign() <= 0 {
		t.Errorf("AddWork() returned non-positive work: %v", result)
	}

	// Initial should not be modified
	if initial.Sign() != 0 {
		t.Errorf("AddWork() modified initial value: %v", initial)
	}
}

func TestCompareChainWork(t *testing.T) {
	a := big.NewInt(100)
	b := big.NewInt(200)
	c := big.NewInt(100)

	if CompareChainWork(a, b) >= 0 {
		t.Errorf("CompareChainWork(100, 200) should be negative")
	}

	if CompareChainWork(b, a) <= 0 {
		t.Errorf("CompareChainWork(200, 100) should be positive")
	}

	if CompareChainWork(a, c) != 0 {
		t.Errorf("CompareChainWork(100, 100) should be zero")
	}
}

func TestChainWorkToHex(t *testing.T) {
	work := big.NewInt(12345)
	hex := ChainWorkToHex(work)

	if len(hex) != 64 {
		t.Errorf("ChainWorkToHex() returned %d characters, expected 64", len(hex))
	}

	// Should be padded with leading zeros
	if hex[0:60] != "000000000000000000000000000000000000000000000000000000000000" {
		t.Errorf("ChainWorkToHex() not properly padded: %s", hex)
	}
}

func TestChainWorkFromHex(t *testing.T) {
	tests := []struct {
		name    string
		hexStr  string
		wantErr bool
	}{
		{
			name:    "valid hex",
			hexStr:  "0000000000000000000000000000000000000000000000000000000000003039",
			wantErr: false,
		},
		{
			name:    "invalid hex",
			hexStr:  "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ChainWorkFromHex(tt.hexStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ChainWorkFromHex() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && result == nil {
				t.Errorf("ChainWorkFromHex() returned nil without error")
			}
		})
	}
}
