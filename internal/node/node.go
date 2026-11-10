// Package node creates and runs a libp2p host for tama.
package node

import (
	"fmt"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// DefaultListenAddrs are used by clients when no listen addresses are
// configured. Port 0 asks the operating system for a free port.
var DefaultListenAddrs = []string{
	"/ip4/0.0.0.0/tcp/0",
	"/ip4/0.0.0.0/udp/0/quic-v1",
}

// DefaultRelayListenAddrs are used when running in relay mode with no explicit
// listen addresses. A relay needs a stable, reachable port.
var DefaultRelayListenAddrs = []string{
	"/ip4/0.0.0.0/tcp/4001",
	"/ip4/0.0.0.0/udp/4001/quic-v1",
}

// Options configures the libp2p host.
type Options struct {
	// ListenAddrs overrides the default listen addresses.
	ListenAddrs []string

	// Relay is a static relay candidate for AutoRelay. It lets a client behind
	// NAT obtain a circuit-relay address.
	Relay *peer.AddrInfo

	// RelayService makes this host act as a circuit relay for other peers.
	RelayService bool
}

// New builds a libp2p host using the given private key as its identity.
func New(priv crypto.PrivKey, options Options) (host.Host, error) {
	if priv == nil {
		return nil, fmt.Errorf("node: private key is required")
	}

	listenAddrs := options.ListenAddrs
	if len(listenAddrs) == 0 {
		listenAddrs = DefaultListenAddrs
	}

	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(listenAddrs...),
		libp2p.UserAgent(AgentVersion),
	}
	if options.RelayService {
		opts = append(opts, libp2p.EnableRelayService())
	}
	if options.Relay != nil {
		opts = append(opts, libp2p.EnableAutoRelayWithStaticRelays([]peer.AddrInfo{*options.Relay}))
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}
	return h, nil
}
