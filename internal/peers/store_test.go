package peers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func testPeerID(t *testing.T) peer.ID {
	t.Helper()
	_, pub, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("derive peer ID: %v", err)
	}
	return id
}

func TestLoadMissingFile(t *testing.T) {
	store, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.Len() != 0 {
		t.Fatalf("Len = %d, want 0", store.Len())
	}
}

func TestLoadAndDedupe(t *testing.T) {
	a := testPeerID(t)
	b := testPeerID(t)

	path := filepath.Join(t.TempDir(), "peers.json")
	data := []byte(`["` + a.String() + `","  ` + b.String() + `  ","` + a.String() + `",""]`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write peers file: %v", err)
	}

	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.Len() != 2 {
		t.Fatalf("Len = %d, want 2", store.Len())
	}
	if !store.Contains(a) || !store.Contains(b) {
		t.Fatalf("store does not contain both peers: %v", store.IDs())
	}

	ids := store.IDs()
	ids[0] = ""
	if !store.Contains(a) {
		t.Fatal("mutating the returned slice changed the store")
	}
}

func TestLoadRejectsInvalidPeer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	if err := os.WriteFile(path, []byte(`["not-a-peer"]`), 0o600); err != nil {
		t.Fatalf("write peers file: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an invalid peer ID")
	}
}

func TestAdd(t *testing.T) {
	store, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	a := testPeerID(t)
	store.Add(a, a)
	if store.Len() != 1 {
		t.Fatalf("Len = %d, want 1 after duplicate Add", store.Len())
	}
	if !store.Contains(a) {
		t.Fatal("Contains = false after Add")
	}
}
