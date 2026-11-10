package node

import (
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

// MDNSServiceName scopes local discovery to tama. It follows the
// "_service._proto" convention required by mDNS.
const MDNSServiceName = "_tama._udp"

type mdnsNotifee struct {
	onPeer func(peer.AddrInfo)
}

func (n *mdnsNotifee) HandlePeerFound(info peer.AddrInfo) {
	if n.onPeer != nil {
		n.onPeer(info)
	}
}

// StartMDNS advertises this host on the local network and reports every peer
// found through onPeer. The returned closer stops the service.
func StartMDNS(h host.Host, onPeer func(peer.AddrInfo)) (io.Closer, error) {
	service := mdns.NewMdnsService(h, MDNSServiceName, &mdnsNotifee{onPeer: onPeer})
	if err := service.Start(); err != nil {
		return nil, fmt.Errorf("start mdns: %w", err)
	}
	return service, nil
}
