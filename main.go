package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/FlexTether/tama/internal/config"
	"github.com/FlexTether/tama/internal/node"
	"github.com/FlexTether/tama/internal/peers"
	"github.com/FlexTether/tama/internal/wsrelay"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func main() {
	if err := run(); err != nil {
		slog.Error("tama failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configDir  = flag.String("config", "", "data directory for identity, peer, database and UI files (default: ./data)")
		listen     = flag.String("listen", "", "comma-separated multiaddrs to listen on")
		relayMode  = flag.Bool("relay", false, "run as a public circuit relay")
		enableMDNS = flag.Bool("mdns", true, "discover known peers on the local network")
	)
	var cliPeers peerList
	flag.Var(&cliPeers, "peer", "known peer ID to connect to (repeatable)")
	flag.Parse()

	env, err := config.LoadEnv(config.EnvFileName)
	if err != nil {
		return err
	}
	slog.Info("loaded environment",
		"web_port", env.WebPort,
		"ws_relay", env.WSRelay,
		"libp2p_relay", env.Libp2pRelay,
	)

	cfg, err := config.New(*configDir)
	if err != nil {
		return err
	}
	slog.Info("using data directory", "dir", cfg.Dir)

	known, err := peers.Load(cfg.KnownPeersPath())
	if err != nil {
		return err
	}
	for _, raw := range cliPeers {
		id, err := peer.Decode(raw)
		if err != nil {
			return fmt.Errorf("parse -peer %q: %w", raw, err)
		}
		known.Add(id)
	}
	slog.Info("loaded known peers", "count", known.Len(), "file", cfg.KnownPeersPath())

	identity, priv, created, err := node.LoadOrCreateIdentity(cfg.IdentityPath())
	if err != nil {
		return err
	}
	if created {
		slog.Info("generated new identity", "peer_id", identity.PeerID, "file", cfg.IdentityPath())
	} else {
		slog.Info("loaded existing identity", "peer_id", identity.PeerID, "file", cfg.IdentityPath())
	}

	listenAddrs, err := parseListenAddrs(*listen, *relayMode)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var relay *peer.AddrInfo
	if !*relayMode {
		relay, err = node.ParseRelay(env.Libp2pRelay)
		if err != nil {
			return err
		}
	}

	options := node.Options{
		ListenAddrs:  listenAddrs,
		Relay:        relay,
		RelayService: *relayMode,
	}
	h, err := node.New(priv, options)
	if err != nil {
		return err
	}
	defer func() {
		if err := h.Close(); err != nil {
			slog.Warn("failed to close libp2p host", "error", err)
		}
	}()

	tracker := newPeerTracker(h)
	info := node.Info(h)

	if *relayMode {
		slog.Info("circuit relay started", "peer_id", info.PeerID)
		for _, addr := range info.FullAddrs {
			slog.Info("relay address (use as LIBP2P_RELAY)", "addr", addr)
		}
	} else {
		if relay != nil {
			if err := node.ConnectToRelay(ctx, h, *relay); err != nil {
				return err
			}
			slog.Info("connected to relay", "peer_id", relay.ID, "addrs", relay.Addrs)
		}

		if *enableMDNS {
			mdnsService, err := node.StartMDNS(h, func(found peer.AddrInfo) {
				if !known.Contains(found.ID) {
					return
				}
				go tracker.connect(ctx, found, "mdns")
			})
			if err != nil {
				slog.Warn("mDNS discovery failed", "error", err)
			} else {
				defer mdnsService.Close()
				slog.Info("mDNS discovery started", "service", node.MDNSServiceName)
			}
		}

		if env.WSRelay != "" {
			wsClient, err := wsrelay.New(env.WSRelay, h, priv)
			if err != nil {
				return err
			}
			go func() {
				if err := wsClient.Run(ctx); err != nil {
					slog.Warn("ws relay stopped", "error", err)
				}
			}()
			go runPeerLookups(ctx, wsClient, known, tracker)
		} else if known.Len() > 0 {
			slog.Warn("WS_RELAY is empty; known peers cannot be looked up over the internet")
		}
	}

	slog.Info("libp2p node started",
		"peer_id", info.PeerID,
		"protocols", len(info.Protocols),
	)
	for _, addr := range info.FullAddrs {
		slog.Info("listening", "addr", addr)
	}

	<-ctx.Done()
	fmt.Println()
	slog.Info("shutting down")
	return nil
}

func parseListenAddrs(raw string, relayMode bool) ([]string, error) {
	var addrs []string
	if raw != "" {
		for _, addr := range strings.Split(raw, ",") {
			if addr = strings.TrimSpace(addr); addr != "" {
				addrs = append(addrs, addr)
			}
		}
	}
	if len(addrs) > 0 {
		return addrs, nil
	}
	if relayMode {
		return node.DefaultRelayListenAddrs, nil
	}
	return node.DefaultListenAddrs, nil
}

// peerTracker dials peers reported by LAN discovery and avoids noisy repeated
// logs for the same peer.
type peerTracker struct {
	h     host.Host
	mu    sync.Mutex
	known map[peer.ID]struct{}
}

func newPeerTracker(h host.Host) *peerTracker {
	return &peerTracker{h: h, known: make(map[peer.ID]struct{})}
}

// connected reports whether this host already has a live connection to id.
func (t *peerTracker) connected(id peer.ID) bool {
	return t.h.Network().Connectedness(id) == network.Connected
}

func (t *peerTracker) connect(ctx context.Context, info peer.AddrInfo, source string) {
	if info.ID == t.h.ID() {
		return
	}
	if t.connected(info.ID) {
		return
	}

	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	err := t.h.Connect(connectCtx, info)
	cancel()
	if err != nil {
		slog.Warn("connect to known peer failed", "source", source, "peer_id", info.ID, "error", err)
		return
	}

	t.mu.Lock()
	_, seen := t.known[info.ID]
	t.known[info.ID] = struct{}{}
	t.mu.Unlock()

	if !seen {
		slog.Info("connected to peer", "source", source, "peer_id", info.ID, "addrs", info.Addrs)
	}
}

// lookupInterval is how often known peers are re-queried over the WS relay.
const lookupInterval = 30 * time.Second

// runPeerLookups queries the WS relay for each known peer immediately and then
// every lookupInterval. Missing responses mean the peer is offline or not
// joined to the relay, so it is simply skipped until the next round.
func runPeerLookups(ctx context.Context, client *wsrelay.Client, known *peers.Store, tracker *peerTracker) {
	if known.Len() == 0 {
		return
	}

	lookupAll := func() {
		var wg sync.WaitGroup
		for _, id := range known.IDs() {
			if id == tracker.h.ID() || tracker.connected(id) {
				continue
			}
			wg.Add(1)
			go func(id peer.ID) {
				defer wg.Done()

				info, err := client.Lookup(ctx, id)
				if err != nil {
					slog.Debug("ws relay: peer lookup failed", "peer_id", id, "error", err)
					return
				}
				tracker.connect(ctx, info, "ws-relay")
			}(id)
		}
		wg.Wait()
	}

	lookupAll()

	ticker := time.NewTicker(lookupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lookupAll()
		}
	}
}

// peerList is a repeatable -peer flag.
type peerList []string

func (p *peerList) String() string { return strings.Join(*p, ",") }

func (p *peerList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("empty peer ID")
	}
	*p = append(*p, value)
	return nil
}
