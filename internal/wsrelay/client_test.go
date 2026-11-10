package wsrelay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlexTether/tama/internal/node"
	"github.com/gorilla/websocket"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "wss://ws.example.com", want: "wss://ws.example.com"},
		{in: "wss://ws.example.com/", want: "wss://ws.example.com"},
		{in: "wss://ws.example.com/tama/", want: "wss://ws.example.com/tama"},
		{in: "ws://127.0.0.1:8080", want: "ws://127.0.0.1:8080"},
		{in: "wss//ws.example.com/", want: "wss://ws.example.com"},
		{in: "ws//localhost:9000/room", want: "ws://localhost:9000/room"},
		{in: "  wss://ws.example.com  ", want: "wss://ws.example.com"},
		{in: "", wantErr: true},
		{in: "https://ws.example.com", wantErr: true},
		{in: "wss://", wantErr: true},
	}

	for _, tc := range tests {
		got, err := normalizeBaseURL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeBaseURL(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeBaseURL(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// testHub is a minimal per-peer-room WebSocket relay: a message received on a
// room is forwarded to every other connection in that room.
type testHub struct {
	mu    sync.Mutex
	rooms map[string]map[*websocket.Conn]struct{}
}

func newTestHub() *testHub {
	return &testHub{rooms: make(map[string]map[*websocket.Conn]struct{})}
}

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

func (h *testHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := testUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	room := r.URL.Path
	h.mu.Lock()
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[*websocket.Conn]struct{})
	}
	h.rooms[room][conn] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.rooms[room], conn)
		if len(h.rooms[room]) == 0 {
			delete(h.rooms, room)
		}
		h.mu.Unlock()
		conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		h.mu.Lock()
		for other := range h.rooms[room] {
			if other == conn {
				continue
			}
			_ = other.WriteMessage(websocket.TextMessage, data)
		}
		h.mu.Unlock()
	}
}

func startTestHost(t *testing.T) (host.Host, crypto.PrivKey) {
	t.Helper()
	_, priv, err := node.NewIdentity()
	if err != nil {
		t.Fatalf("new identity: %v", err)
	}
	h, err := node.New(priv, node.Options{})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h, priv
}

func TestLookupThroughRoomRelay(t *testing.T) {
	hub := newTestHub()
	server := httptest.NewServer(hub)
	defer server.Close()

	base := "ws" + strings.TrimPrefix(server.URL, "http")

	hA, privA := startTestHost(t)
	hB, privB := startTestHost(t)

	clientA, err := New(base, hA, privA)
	if err != nil {
		t.Fatalf("client A: %v", err)
	}
	clientB, err := New(base, hB, privB)
	if err != nil {
		t.Fatalf("client B: %v", err)
	}
	clientB.timeout = 3 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go clientA.Run(ctx)

	// Give A time to join its own room.
	time.Sleep(200 * time.Millisecond)

	info, err := clientB.Lookup(ctx, hA.ID())
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info.ID != hA.ID() {
		t.Fatalf("looked up %s, got %s", hA.ID(), info.ID)
	}
	if len(info.Addrs) == 0 {
		t.Fatal("response contained no addresses")
	}
}

func TestLookupOfflinePeerTimesOut(t *testing.T) {
	hub := newTestHub()
	server := httptest.NewServer(hub)
	defer server.Close()

	base := "ws" + strings.TrimPrefix(server.URL, "http")

	h, priv := startTestHost(t)
	client, err := New(base, h, priv)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	client.timeout = 300 * time.Millisecond

	offline, _, err := node.NewIdentity()
	if err != nil {
		t.Fatalf("new identity: %v", err)
	}
	offlineID, err := peer.Decode(offline.PeerID)
	if err != nil {
		t.Fatalf("decode peer ID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := client.Lookup(ctx, offlineID); err == nil {
		t.Fatal("expected lookup of an offline peer to time out")
	}
}
