package node

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// ParseRelay parses a relay multiaddr such as
//
//	/ip4/203.0.113.10/tcp/4001/p2p/12D3KooW...
//
// The /p2p/<peer-id> component is required: without it there is no way to
// verify which peer the relay is. An empty string returns (nil, nil), meaning
// "no relay configured".
func ParseRelay(raw string) (*peer.AddrInfo, error) {
	if raw == "" {
		return nil, nil
	}

	addr, err := multiaddr.NewMultiaddr(raw)
	if err != nil {
		return nil, fmt.Errorf("parse relay multiaddr %q: %w", raw, err)
	}

	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return nil, fmt.Errorf("relay multiaddr %q must end with /p2p/<peer-id>: %w", raw, err)
	}
	return info, nil
}

// ConnectToRelay dials the relay and waits for the connection to be
// established, bounded by the context deadline.
func ConnectToRelay(ctx context.Context, h host.Host, relay peer.AddrInfo) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := h.Connect(ctx, relay); err != nil {
		return fmt.Errorf("connect to relay %s: %w", relay.ID, err)
	}
	return nil
}
