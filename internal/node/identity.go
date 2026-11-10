package node

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Identity is the persisted form of a libp2p node identity.
//
// The private key is stored in libp2p's protobuf-serialized form, base64
// encoded so the file stays valid JSON. The peer ID is stored alongside it so
// the file is human-readable; it is derived from the key and verified on load.
type Identity struct {
	PeerID     string    `json:"peer_id"`
	PrivateKey string    `json:"private_key"`
	CreatedAt  time.Time `json:"created_at"`
}

// LoadOrCreateIdentity loads the identity stored at path. If the file does not
// exist, a new Ed25519 identity is generated and written to disk. The returned
// bool reports whether a new identity was created.
func LoadOrCreateIdentity(path string) (*Identity, crypto.PrivKey, bool, error) {
	identity, key, err := loadIdentity(path)
	switch {
	case err == nil:
		return identity, key, false, nil
	case errors.Is(err, fs.ErrNotExist):
		// fall through and generate a new identity
	default:
		return nil, nil, false, err
	}

	identity, key, err = NewIdentity()
	if err != nil {
		return nil, nil, false, err
	}
	if err := SaveIdentity(path, identity); err != nil {
		return nil, nil, false, err
	}
	return identity, key, true, nil
}

func loadIdentity(path string) (*Identity, crypto.PrivKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read identity %q: %w", path, err)
	}

	var identity Identity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, nil, fmt.Errorf("parse identity %q: %w", path, err)
	}

	key, err := identity.PrivKey()
	if err != nil {
		return nil, nil, fmt.Errorf("parse identity %q: %w", path, err)
	}

	// Make sure the cached peer ID still matches the key.
	if identity.PeerID != "" {
		derived, err := peer.IDFromPrivateKey(key)
		if err != nil {
			return nil, nil, fmt.Errorf("derive peer ID: %w", err)
		}
		if derived.String() != identity.PeerID {
			return nil, nil, fmt.Errorf(
				"identity %q is inconsistent: peer_id %q does not match private key (got %q)",
				path, identity.PeerID, derived,
			)
		}
	} else {
		derived, err := peer.IDFromPrivateKey(key)
		if err != nil {
			return nil, nil, fmt.Errorf("derive peer ID: %w", err)
		}
		identity.PeerID = derived.String()
	}

	return &identity, key, nil
}

// NewIdentity generates a fresh Ed25519 key pair and derives its peer ID.
func NewIdentity() (*Identity, crypto.PrivKey, error) {
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("derive peer ID: %w", err)
	}

	raw, err := crypto.MarshalPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}

	return &Identity{
		PeerID:     id.String(),
		PrivateKey: base64.StdEncoding.EncodeToString(raw),
		CreatedAt:  time.Now().UTC(),
	}, key, nil
}

// PrivKey decodes and returns the private key stored in the identity.
func (i *Identity) PrivKey() (crypto.PrivKey, error) {
	raw, err := base64.StdEncoding.DecodeString(i.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	key, err := crypto.UnmarshalPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal private key: %w", err)
	}
	return key, nil
}

// SaveIdentity writes the identity to path with 0600 permissions. The write is
// atomic: data goes to a temporary file in the same directory which is then
// renamed over path.
func SaveIdentity(path string, identity *Identity) error {
	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create identity dir %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".identity-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp identity file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp identity file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp identity file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp identity file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp identity file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("save identity %q: %w", path, err)
	}
	return nil
}
