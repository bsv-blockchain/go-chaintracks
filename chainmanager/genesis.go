package chainmanager

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/bsv-blockchain/go-chaincfg"
	"github.com/bsv-blockchain/go-sdk/block"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

// Network name constants used throughout the package. Legacy short names
// ("main", "test", "teratest") are accepted as aliases for the canonical
// go-chaincfg names ("mainnet", "testnet", "teratestnet") so existing
// configuration, on-disk filenames, and CDN URLs keep working.
const (
	networkMain        = "main"
	networkTest        = "test"
	networkTeratest    = "teratest"
	networkTeratestnet = "teratestnet"
	networkRegtest     = "regtest"
)

// networkAliases maps legacy chaintracks network names to the canonical
// names used by go-chaincfg. Canonical names pass through unchanged.
//
//nolint:gochecknoglobals // immutable alias table
var networkAliases = map[string]string{
	networkMain:     "mainnet",
	networkTest:     "testnet",
	networkTeratest: "teratestnet",
}

// getGenesisHeader returns the genesis block header for the given network,
// resolving the name through go-chaincfg. Every network exported by
// chaincfg.GetChainParams is supported.
func getGenesisHeader(network string) (*block.Header, error) {
	canonical := network
	if alias, ok := networkAliases[network]; ok {
		canonical = alias
	}

	params, err := chaincfg.GetChainParams(canonical)
	if err != nil {
		if errors.Is(err, chaincfg.ErrUnknownNetwork) {
			return nil, fmt.Errorf("%w: %s", chaintracks.ErrUnknownNetwork, network)
		}
		return nil, fmt.Errorf("get chain params for %s: %w", network, err)
	}

	var buf bytes.Buffer
	if err := params.GenesisBlock.Header.Serialize(&buf); err != nil {
		return nil, fmt.Errorf("serialize genesis header for %s: %w", network, err)
	}
	return block.NewHeaderFromBytes(buf.Bytes())
}
