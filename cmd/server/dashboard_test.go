package main

import (
	"strings"
	"testing"

	msgbus "github.com/bsv-blockchain/go-p2p-message-bus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardHandlerRenderPeerList(t *testing.T) {
	tests := []struct {
		name              string
		peers             []msgbus.PeerInfo
		expectContains    []string
		expectNotContains []string
	}{
		{
			name:  "EmptyPeerList",
			peers: []msgbus.PeerInfo{},
			expectContains: []string{
				"No peers connected",
				"color: #808080",
				"font-style: italic",
			},
			expectNotContains: []string{
				"<div class=\"peer\">",
			},
		},
		{
			name:  "NilPeerList",
			peers: nil,
			expectContains: []string{
				"No peers connected",
				"color: #808080",
			},
			expectNotContains: []string{
				"<div class=\"peer\">",
			},
		},
		{
			name: "SinglePeerWithValidName",
			peers: []msgbus.PeerInfo{
				{
					ID:    "QmPeer12345",
					Name:  "TestNode",
					Addrs: []string{"/ip4/192.168.1.1/tcp/4001"},
				},
			},
			expectContains: []string{
				"<div class=\"peer\">",
				"<strong>TestNode</strong>",
				"QmPeer12345",
				`<div class="peer-id">QmPeer12345</div>`,
				`<div class="peer-addr">/ip4/192.168.1.1/tcp/4001</div>`,
			},
			expectNotContains: []string{
				"No peers connected",
				"Unknown Peer",
			},
		},
		{
			name: "MultiplePeers",
			peers: []msgbus.PeerInfo{
				{
					ID:    "QmPeer1",
					Name:  "NodeOne",
					Addrs: []string{"/ip4/192.168.1.1/tcp/4001"},
				},
				{
					ID:    "QmPeer2",
					Name:  "NodeTwo",
					Addrs: []string{"/ip4/192.168.1.2/tcp/4001"},
				},
				{
					ID:    "QmPeer3",
					Name:  "NodeThree",
					Addrs: []string{"/ip4/192.168.1.3/tcp/4001"},
				},
			},
			expectContains: []string{
				"<strong>NodeOne</strong>",
				"<strong>NodeTwo</strong>",
				"<strong>NodeThree</strong>",
				"QmPeer1",
				"QmPeer2",
				"QmPeer3",
			},
			expectNotContains: []string{
				"No peers connected",
			},
		},
		{
			name: "PeerWithUnknownName",
			peers: []msgbus.PeerInfo{
				{
					ID:    "QmPeer123",
					Name:  "unknown",
					Addrs: []string{"/ip4/192.168.1.1/tcp/4001"},
				},
			},
			expectContains: []string{
				"<strong>Unknown Peer</strong>",
				"QmPeer123",
			},
			expectNotContains: []string{
				"<strong>unknown</strong>",
			},
		},
		{
			name: "PeerWithEmptyName",
			peers: []msgbus.PeerInfo{
				{
					ID:    "QmPeer456",
					Name:  "",
					Addrs: []string{"/ip4/192.168.1.1/tcp/4001"},
				},
			},
			expectContains: []string{
				"<strong>Unknown Peer</strong>",
				"QmPeer456",
			},
			expectNotContains: []string{
				"<strong></strong>",
			},
		},
		{
			name: "PeerWithMultipleAddresses",
			peers: []msgbus.PeerInfo{
				{
					ID:   "QmPeerMulti",
					Name: "MultiAddrNode",
					Addrs: []string{
						"/ip4/192.168.1.1/tcp/4001",
						"/ip6/::1/tcp/4001",
						"/dns4/example.com/tcp/4001",
					},
				},
			},
			expectContains: []string{
				"<strong>MultiAddrNode</strong>",
				"QmPeerMulti",
				`<div class="peer-addr">/ip4/192.168.1.1/tcp/4001</div>`,
				`<div class="peer-addr">/ip6/::1/tcp/4001</div>`,
				`<div class="peer-addr">/dns4/example.com/tcp/4001</div>`,
			},
			expectNotContains: nil,
		},
		{
			name: "PeerWithNoAddresses",
			peers: []msgbus.PeerInfo{
				{
					ID:    "QmPeerNoAddr",
					Name:  "NoAddressNode",
					Addrs: []string{},
				},
			},
			expectContains: []string{
				"<strong>NoAddressNode</strong>",
				"QmPeerNoAddr",
			},
			expectNotContains: []string{
				"<div class=\"peer-addr\">",
			},
		},
		{
			name: "MixedPeerNames",
			peers: []msgbus.PeerInfo{
				{
					ID:    "QmPeer1",
					Name:  "ValidName",
					Addrs: []string{"/ip4/192.168.1.1/tcp/4001"},
				},
				{
					ID:    "QmPeer2",
					Name:  "unknown",
					Addrs: []string{"/ip4/192.168.1.2/tcp/4001"},
				},
				{
					ID:    "QmPeer3",
					Name:  "",
					Addrs: []string{"/ip4/192.168.1.3/tcp/4001"},
				},
			},
			expectContains: []string{
				"<strong>ValidName</strong>",
				"<strong>Unknown Peer</strong>",
				"QmPeer1",
				"QmPeer2",
				"QmPeer3",
			},
			expectNotContains: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &DashboardHandler{}

			result := handler.renderPeerList(tt.peers)

			require.NotEmpty(t, result, "renderPeerList should not return empty string")

			// Check for expected content
			for _, expected := range tt.expectContains {
				assert.Contains(t, result, expected,
					"Expected result to contain: %s", expected)
			}

			// Check for unexpected content
			for _, notExpected := range tt.expectNotContains {
				assert.NotContains(t, result, notExpected,
					"Expected result to NOT contain: %s", notExpected)
			}

			// Additional validation for empty peer list
			if len(tt.peers) == 0 {
				// Should return exactly the "no peers" message
				assert.Equal(t, `<div style="color: #808080; font-style: italic;">No peers connected</div>`, result)
			}

			// Additional validation for non-empty peer list
			if len(tt.peers) > 0 {
				// Should contain a peer div for each peer
				peerDivCount := strings.Count(result, `<div class="peer">`)
				assert.Equal(t, len(tt.peers), peerDivCount,
					"Should have one peer div for each peer")
			}
		})
	}
}
