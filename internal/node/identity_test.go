package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")

	first, key, created, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatalf("first LoadOrCreateIdentity: %v", err)
	}
	if !created {
		t.Fatal("expected first call to create an identity")
	}
	if first.PeerID == "" || key == nil {
		t.Fatal("expected peer ID and private key")
	}

	perm, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat identity: %v", err)
	}
	if got := perm.Mode().Perm(); got != 0o600 {
		t.Fatalf("identity permissions = %o, want 600", got)
	}

	second, key2, created, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatalf("second LoadOrCreateIdentity: %v", err)
	}
	if created {
		t.Fatal("expected second call to load the existing identity")
	}
	if second.PeerID != first.PeerID {
		t.Fatalf("peer ID changed: %q != %q", second.PeerID, first.PeerID)
	}
	if !key.Equals(key2) {
		t.Fatal("private key changed between loads")
	}
}

func TestLoadOrCreateIdentityRejectsMismatchedPeerID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")

	identity, _, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	identity.PeerID = "12D3KooWNotTheRightPeerID"
	if err := SaveIdentity(path, identity); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}

	if _, _, _, err := LoadOrCreateIdentity(path); err == nil {
		t.Fatal("expected an error for a peer ID that does not match the key")
	}
}
