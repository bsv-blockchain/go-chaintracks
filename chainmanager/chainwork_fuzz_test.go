package chainmanager

import (
	"testing"
)

// FuzzCompactToBig tests CompactToBig with random uint32 values
// to ensure it never panics and always returns a valid big.Int.
func FuzzCompactToBig(f *testing.F) {
	// Seed corpus with known interesting values
	f.Add(uint32(0x1d00ffff)) // genesis block mainnet
	f.Add(uint32(0x1b0404cb)) // typical difficulty
	f.Add(uint32(0x00000000)) // zero
	f.Add(uint32(0xffffffff)) // max uint32
	f.Add(uint32(0x01000000)) // exponent = 1
	f.Add(uint32(0x02000000)) // exponent = 2
	f.Add(uint32(0x03000000)) // exponent = 3
	f.Add(uint32(0x00800000)) // sign bit set, exponent = 0
	f.Add(uint32(0x01800000)) // sign bit set, exponent = 1

	f.Fuzz(func(t *testing.T, compact uint32) {
		// Should never panic
		result := CompactToBig(compact)

		// Result should never be nil
		if result == nil {
			t.Fatal("CompactToBig returned nil")
		}

		// Result should be a valid big.Int
		if result.BitLen() < 0 {
			t.Errorf("CompactToBig(%x) returned invalid big.Int", compact)
		}

		// Verify sign bit handling
		isNegative := compact&0x00800000 != 0
		if isNegative && result.Sign() > 0 {
			t.Errorf("CompactToBig(%x) should be negative but got sign %d", compact, result.Sign())
		}

		// For zero mantissa, result should be zero
		mantissa := compact & 0x007fffff
		if mantissa == 0 && result.Sign() != 0 {
			t.Errorf("CompactToBig(%x) with zero mantissa should return zero, got %v", compact, result)
		}
	})
}

// FuzzChainWorkFromHex tests ChainWorkFromHex with random strings
// to ensure it handles invalid input gracefully without panicking.
func FuzzChainWorkFromHex(f *testing.F) {
	// Seed corpus with valid and invalid hex strings
	f.Add("0000000000000000000000000000000000000000000000000000000000003039")
	f.Add("1234567890abcdef")
	f.Add("invalid")
	f.Add("")
	f.Add("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	f.Add("000000000000000000000000000000000000000000000000000000000000000g") // invalid hex char
	f.Add("0x1234")                                                           // 0x prefix
	f.Add("   1234   ")                                                       // whitespace
	f.Add("\n1234\n")                                                         // newlines
	f.Add("😀")                                                                // unicode
	f.Add(string([]byte{0x00, 0x01, 0x02}))                                   // control chars

	f.Fuzz(func(t *testing.T, hexStr string) {
		// Skip unrealistically long inputs to avoid timeout on expensive big.Int operations
		// Bitcoin chainwork is at most 256 bits = 64 hex characters
		if len(hexStr) > 64 {
			return
		}

		// Should never panic
		result, err := ChainWorkFromHex(hexStr)

		// If no error, result should not be nil
		if err == nil && result == nil {
			t.Fatal("ChainWorkFromHex returned nil without error")
		}

		// If error, result should be nil
		if err != nil && result != nil {
			t.Errorf("ChainWorkFromHex returned non-nil result with error: %v", err)
		}

		// If successful, result should be a valid big.Int
		if err == nil {
			if result.BitLen() < 0 {
				t.Errorf("ChainWorkFromHex(%q) returned invalid big.Int", hexStr)
			}

			// Round-trip test: convert back to hex and parse again
			hexOut := result.Text(16)
			result2, err2 := ChainWorkFromHex(hexOut)
			if err2 != nil {
				t.Errorf("Round-trip failed for %q: %v", hexStr, err2)
			}
			if result.Cmp(result2) != 0 {
				t.Errorf("Round-trip mismatch for %q: %v != %v", hexStr, result, result2)
			}
		}
	})
}
