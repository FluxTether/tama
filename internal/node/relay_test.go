package node

import (
	"crypto/rand"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestParseRelayEmpty(t *testing.T) {
	info, err := ParseRelay("")
	if err != nil {
		t.Fatalf("ParseRelay: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil relay for empty input, got %+v", info)
	}
}

func TestParseRelay(t *testing.T) {
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("derive peer ID: %v", err)
	}

	info, err := ParseRelay("/ip4/127.0.0.1/tcp/4001/p2p/" + id.String())
	if err != nil {
		t.Fatalf("ParseRelay: %v", err)
	}
	if info.ID != id {
		t.Fatalf("peer ID = %s, want %s", info.ID, id)
	}
	if len(info.Addrs) != 1 {
		t.Fatalf("addrs = %v, want one address", info.Addrs)
	}
}

func TestParseRelayRequiresPeerID(t *testing.T) {
	if _, err := ParseRelay("/ip4/127.0.0.1/tcp/4001"); err == nil {
		t.Fatal("expected an error for a relay multiaddr without /p2p/<peer-id>")
	}
}

func TestParseRelayRejectsInvalidMultiaddr(t *testing.T) {
	if _, err := ParseRelay("not-a-multiaddr"); err == nil {
		t.Fatal("expected an error for an invalid multiaddr")
	}
}
