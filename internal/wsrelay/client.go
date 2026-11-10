// Package wsrelay implements tama's WebSocket message relay client.
//
// The relay is a dumb message bus with per-peer rooms: each node joins the room
// named after its own peer ID and answers "query" messages. To look a peer up,
// a node joins the target's room, sends a signed query, and waits for a signed
// response containing the target's current multiaddrs. Presence is inferred
// from the response (or its absence); the relay itself reports no status.
package wsrelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/FlexTether/tama/internal/node"
	"github.com/gorilla/websocket"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// DefaultQueryTimeout is how long Lookup waits for a signed response.
const DefaultQueryTimeout = 6 * time.Second

// Client joins the relay room for its own peer ID and performs lookups for
// known peers.
type Client struct {
	baseURL string
	h       host.Host
	priv    crypto.PrivKey
	own     peer.ID
	timeout time.Duration

	writeMu sync.Mutex
}

// New validates rawURL and creates a client. rawURL is the relay base URL, e.g.
// wss://ws.tridentlab.in or wss://ws.tridentlab.in/tama.
func New(rawURL string, h host.Host, priv crypto.PrivKey) (*Client, error) {
	if h == nil {
		return nil, errors.New("wsrelay: host is required")
	}
	if priv == nil {
		return nil, errors.New("wsrelay: private key is required")
	}

	base, err := normalizeBaseURL(rawURL)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL: base,
		h:       h,
		priv:    priv,
		own:     h.ID(),
		timeout: DefaultQueryTimeout,
	}, nil
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("wsrelay: relay URL is empty")
	}

	// Tolerate the common "wss//host" typo.
	switch {
	case strings.HasPrefix(raw, "wss//"):
		raw = "wss://" + strings.TrimPrefix(raw, "wss//")
	case strings.HasPrefix(raw, "ws//"):
		raw = "ws://" + strings.TrimPrefix(raw, "ws//")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("wsrelay: parse URL %q: %w", raw, err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return "", fmt.Errorf("wsrelay: URL %q must use ws:// or wss://", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("wsrelay: URL %q has no host", raw)
	}

	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

func (c *Client) roomURL(id peer.ID) string {
	return c.baseURL + "/" + id.String()
}

// Run keeps a connection to this node's own room and answers queries until ctx
// is canceled. It reconnects with exponential backoff.
func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second

	for {
		started := time.Now()
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}

		if time.Since(started) > time.Minute {
			backoff = time.Second
		}
		slog.Warn("ws relay connection lost",
			"room", c.roomURL(c.own),
			"error", err,
			"retry_in", backoff,
		)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func (c *Client) runOnce(ctx context.Context) error {
	room := c.roomURL(c.own)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, room, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", room, err)
	}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	slog.Info("joined ws relay room", "room", room)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read from %s: %w", room, err)
		}
		c.handleQuery(conn, data)
	}
}

func (c *Client) handleQuery(conn *websocket.Conn, data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("ws relay: invalid message", "error", err)
		return
	}
	if msg.From == c.own.String() || msg.Type != TypeQuery || msg.To != c.own.String() {
		return
	}
	if err := verifyMessage(&msg); err != nil {
		slog.Warn("ws relay: rejected unsigned/invalid query", "from", msg.From, "error", err)
		return
	}
	if !fresh(msg.TS, time.Now()) {
		slog.Warn("ws relay: rejected stale query", "from", msg.From)
		return
	}

	response := &Message{
		Type:  TypeResponse,
		From:  c.own.String(),
		To:    msg.From,
		Nonce: msg.Nonce,
		Addrs: node.FullAddrs(c.h),
		TS:    time.Now().Unix(),
	}
	if err := signMessage(c.priv, response); err != nil {
		slog.Warn("ws relay: failed to sign response", "error", err)
		return
	}
	if err := c.writeMessage(conn, response); err != nil {
		slog.Warn("ws relay: failed to send response", "peer_id", msg.From, "error", err)
		return
	}
	slog.Info("ws relay: answered peer lookup", "peer_id", msg.From)
}

// Lookup joins target's room, sends a signed query and waits for a signed
// response. It returns the target's current addresses; absence of a response
// means the target is offline or not joined to the relay.
func (c *Client) Lookup(ctx context.Context, target peer.ID) (peer.AddrInfo, error) {
	if target == c.own {
		return peer.AddrInfo{}, errors.New("wsrelay: cannot look up self")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	room := c.roomURL(target)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, room, nil)
	if err != nil {
		return peer.AddrInfo{}, fmt.Errorf("dial %s: %w", room, err)
	}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	nonce, err := newNonce()
	if err != nil {
		return peer.AddrInfo{}, err
	}

	query := &Message{
		Type:  TypeQuery,
		From:  c.own.String(),
		To:    target.String(),
		Nonce: nonce,
		TS:    time.Now().Unix(),
	}
	if err := signMessage(c.priv, query); err != nil {
		return peer.AddrInfo{}, err
	}
	if err := c.writeMessage(conn, query); err != nil {
		return peer.AddrInfo{}, fmt.Errorf("send query to %s: %w", target, err)
	}

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return peer.AddrInfo{}, fmt.Errorf("no response from %s: %w", target, ctx.Err())
			}
			return peer.AddrInfo{}, fmt.Errorf("read response from %s: %w", target, err)
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Type != TypeResponse ||
			msg.From != target.String() ||
			msg.To != c.own.String() ||
			msg.Nonce != nonce {
			continue
		}
		if err := verifyMessage(&msg); err != nil {
			slog.Warn("ws relay: rejected invalid response", "peer_id", target, "error", err)
			continue
		}
		if !fresh(msg.TS, time.Now()) {
			continue
		}

		addrs := addrsFromMessage(target, msg.Addrs)
		if len(addrs) == 0 {
			return peer.AddrInfo{}, fmt.Errorf("peer %s responded without usable addresses", target)
		}
		return peer.AddrInfo{ID: target, Addrs: addrs}, nil
	}
}

func (c *Client) writeMessage(conn *websocket.Conn, msg *Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}
