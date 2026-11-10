package node

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
)

// AgentVersion identifies this implementation to other libp2p peers.
const AgentVersion = "tama/0.1.0"

// PeerInfo is the public, shareable description of a running node. It contains
// no secret material and is kept in memory only: listen addresses and ports
// change between runs, so a persisted copy would go stale.
type PeerInfo struct {
	PeerID       string    `json:"peer_id"`
	Addrs        []string  `json:"addrs"`
	FullAddrs    []string  `json:"full_addrs"`
	AgentVersion string    `json:"agent_version"`
	Protocols    []string  `json:"protocols"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Info collects the current public information of a host. The result lives in
// RAM and is regenerated on every start.
func Info(h host.Host) *PeerInfo {
	addrs := h.Addrs()
	addrStrings := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addrStrings = append(addrStrings, addr.String())
	}
	sort.Strings(addrStrings)

	protocols := h.Mux().Protocols()
	protoStrings := make([]string, 0, len(protocols))
	for _, p := range protocols {
		protoStrings = append(protoStrings, string(p))
	}
	sort.Strings(protoStrings)

	return &PeerInfo{
		PeerID:       h.ID().String(),
		Addrs:        addrStrings,
		FullAddrs:    FullAddrs(h),
		AgentVersion: AgentVersion,
		Protocols:    protoStrings,
		UpdatedAt:    time.Now().UTC(),
	}
}

// FullAddrs returns the node's listen addresses with the peer ID appended, i.e.
// addresses other peers can dial directly.
func FullAddrs(h host.Host) []string {
	addrs := h.Addrs()
	full := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		full = append(full, fmt.Sprintf("%s/p2p/%s", addr, h.ID()))
	}
	sort.Strings(full)
	return full
}

// JSON encodes the peer info as indented JSON in memory, ready to be written to
// an HTTP/WebSocket response by the UI without touching the disk.
func (p *PeerInfo) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal peer info: %w", err)
	}
	return append(data, '\n'), nil
}
