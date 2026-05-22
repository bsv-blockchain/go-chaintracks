package chainmanager

import (
	"testing"

	"github.com/bsv-blockchain/go-chaincfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

func TestGetGenesisHeader(t *testing.T) {
	cases := []struct {
		network  string
		wantHash string
	}{
		{networkMain, "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"},
		{"mainnet", "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"},
		{networkTest, "000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943"},
		{"testnet", "000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943"},
		{networkRegtest, "0f9188f13cb7b2c71f2a335e3a4fc328bf5beb436012afca590b1a11466e2206"},
	}
	for _, tc := range cases {
		t.Run(tc.network, func(t *testing.T) {
			h, err := getGenesisHeader(tc.network)
			require.NoError(t, err)
			require.NotNil(t, h)
			assert.Equal(t, tc.wantHash, h.Hash().String())
		})
	}
}

// TestGetGenesisHeader_AllChaincfgNetworks asserts that every network exported
// by go-chaincfg (plus the chaintracks legacy aliases) resolves to a header
// whose hash matches the hash of chaincfg's GenesisBlock — i.e. our
// serialize→parse roundtrip is lossless. We compare against
// GenesisBlock.Header.BlockHash() (computed from the block we serialize),
// not params.GenesisHash, because upstream chaincfg's stn literal is known
// to disagree with its block.
func TestGetGenesisHeader_AllChaincfgNetworks(t *testing.T) {
	cases := []struct {
		network   string
		canonical string
	}{
		{networkMain, "mainnet"},
		{"mainnet", "mainnet"},
		{networkTest, "testnet"},
		{"testnet", "testnet"},
		{networkTeratest, "teratestnet"},
		{networkTeratestnet, "teratestnet"},
		{networkRegtest, "regtest"},
		{"stn", "stn"},
		{"tstn", "tstn"},
	}
	for _, tc := range cases {
		t.Run(tc.network, func(t *testing.T) {
			params, err := chaincfg.GetChainParams(tc.canonical)
			require.NoError(t, err)

			h, err := getGenesisHeader(tc.network)
			require.NoError(t, err)
			require.NotNil(t, h)
			assert.Equal(t, params.GenesisBlock.Header.BlockHash().String(), h.Hash().String())
		})
	}
}

func TestGetGenesisHeader_UnknownNetwork(t *testing.T) {
	_, err := getGenesisHeader("nonsense")
	require.Error(t, err)
	assert.ErrorIs(t, err, chaintracks.ErrUnknownNetwork)
}
