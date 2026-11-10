package wsrelay

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// Message types exchanged over the relay.
const (
	TypeQuery    = "query"
	TypeResponse = "response"
)

// Message is the JSON envelope sent through the WebSocket relay. The relay
// itself does not interpret it; it only forwards messages between room members.
type Message struct {
	Type  string   `json:"type"`
	From  string   `json:"from"`
	To    string   `json:"to,omitempty"`
	Nonce string   `json:"nonce,omitempty"`
	Addrs []string `json:"addrs,omitempty"`
	TS    int64    `json:"ts"`
	Sig   string   `json:"sig"`
}

// payload returns the canonical bytes covered by Sig (everything except Sig).
func (m *Message) payload() ([]byte, error) {
	cp := *m
	cp.Sig = ""
	return json.Marshal(&cp)
}

func signMessage(priv crypto.PrivKey, m *Message) error {
	payload, err := m.payload()
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	sig, err := priv.Sign(payload)
	if err != nil {
		return fmt.Errorf("sign message: %w", err)
	}
	m.Sig = base64.StdEncoding.EncodeToString(sig)
	return nil
}

func verifyMessage(m *Message) error {
	if m.From == "" {
		return errors.New("message has no sender")
	}
	if m.Sig == "" {
		return errors.New("message has no signature")
	}

	id, err := peer.Decode(m.From)
	if err != nil {
		return fmt.Errorf("decode sender %q: %w", m.From, err)
	}
	pub, err := id.ExtractPublicKey()
	if err != nil {
		return fmt.Errorf("extract public key for %s: %w", id, err)
	}

	sig, err := base64.StdEncoding.DecodeString(m.Sig)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	payload, err := m.payload()
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	ok, err := pub.Verify(payload, sig)
	if err != nil {
		return fmt.Errorf("verify signature: %w", err)
	}
	if !ok {
		return errors.New("invalid signature")
	}
	return nil
}

func newNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// fresh reports whether ts is within the accepted clock-skew window.
func fresh(ts int64, now time.Time) bool {
	if ts == 0 {
		return false
	}
	diff := now.Sub(time.Unix(ts, 0))
	return diff > -5*time.Minute && diff < 5*time.Minute
}

// addrsFromMessage parses multiaddrs announced by id. Full addresses
// (".../p2p/<id>") are verified to belong to id; bare multiaddrs are accepted
// as-is. Invalid or mismatched entries are skipped.
func addrsFromMessage(id peer.ID, raw []string) []multiaddr.Multiaddr {
	var out []multiaddr.Multiaddr
	seen := make(map[string]struct{})

	add := func(addr multiaddr.Multiaddr) {
		key := addr.String()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, addr)
	}

	for _, s := range raw {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			continue
		}
		if info, err := peer.AddrInfoFromP2pAddr(addr); err == nil {
			if info.ID != id {
				continue
			}
			for _, a := range info.Addrs {
				add(a)
			}
			continue
		}
		add(addr)
	}
	return out
}
