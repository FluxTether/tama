package wsrelay

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func testKey(t *testing.T) (crypto.PrivKey, peer.ID) {
	t.Helper()
	priv, pub, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("derive peer ID: %v", err)
	}
	return priv, id
}

func TestSignVerifyRoundTrip(t *testing.T) {
	priv, id := testKey(t)

	msg := &Message{
		Type:  TypeQuery,
		From:  id.String(),
		To:    id.String(),
		Nonce: "abc",
		TS:    time.Now().Unix(),
	}
	if err := signMessage(priv, msg); err != nil {
		t.Fatalf("signMessage: %v", err)
	}
	if err := verifyMessage(msg); err != nil {
		t.Fatalf("verifyMessage: %v", err)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	priv, id := testKey(t)
	otherPriv, otherID := testKey(t)

	msg := &Message{
		Type:  TypeResponse,
		From:  id.String(),
		To:    otherID.String(),
		Nonce: "abc",
		TS:    time.Now().Unix(),
	}
	if err := signMessage(priv, msg); err != nil {
		t.Fatalf("signMessage: %v", err)
	}

	msg.Nonce = "changed"
	if err := verifyMessage(msg); err == nil {
		t.Fatal("expected tampered nonce to fail verification")
	}

	// A signature made by a different key must not verify for id.
	msg.Nonce = "abc"
	if err := signMessage(otherPriv, msg); err != nil {
		t.Fatalf("signMessage: %v", err)
	}
	if err := verifyMessage(msg); err == nil {
		t.Fatal("expected signature from another key to fail verification")
	}
}

func TestVerifyRejectsMissingFields(t *testing.T) {
	if err := verifyMessage(&Message{}); err == nil {
		t.Fatal("expected an error for a message without a sender")
	}
	priv, id := testKey(t)
	msg := &Message{From: id.String(), TS: time.Now().Unix()}
	if err := signMessage(priv, msg); err != nil {
		t.Fatalf("signMessage: %v", err)
	}
	msg.Sig = ""
	if err := verifyMessage(msg); err == nil {
		t.Fatal("expected an error for a message without a signature")
	}
}

func TestFresh(t *testing.T) {
	now := time.Now()
	if !fresh(now.Unix(), now) {
		t.Fatal("current timestamp should be fresh")
	}
	if fresh(now.Add(-10*time.Minute).Unix(), now) {
		t.Fatal("old timestamp should be stale")
	}
	if fresh(now.Add(10*time.Minute).Unix(), now) {
		t.Fatal("future timestamp should be stale")
	}
	if fresh(0, now) {
		t.Fatal("zero timestamp should be stale")
	}
}

func TestAddrsFromMessage(t *testing.T) {
	_, id := testKey(t)
	_, otherID := testKey(t)

	addrs := addrsFromMessage(id, []string{
		"/ip4/127.0.0.1/tcp/4001",
		"/ip4/127.0.0.1/tcp/4001",                           // duplicate
		"/ip4/10.0.0.1/udp/4001/quic-v1/p2p/" + id.String(), // matching ID
		"/ip4/10.0.0.2/udp/4001/quic-v1/p2p/" + otherID.String(),
		"not-a-multiaddr",
	})

	if len(addrs) != 2 {
		t.Fatalf("got %d addrs, want 2: %v", len(addrs), addrs)
	}
	want := map[string]bool{
		"/ip4/127.0.0.1/tcp/4001":        true,
		"/ip4/10.0.0.1/udp/4001/quic-v1": true,
	}
	for _, addr := range addrs {
		if !want[addr.String()] {
			t.Fatalf("unexpected address %s", addr)
		}
	}
}

func TestAddrsFromMessageAllInvalid(t *testing.T) {
	_, id := testKey(t)
	if addrs := addrsFromMessage(id, []string{"", "nope"}); len(addrs) != 0 {
		t.Fatalf("got %v, want no addresses", addrs)
	}
}
