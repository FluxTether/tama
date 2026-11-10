// Package peers stores the peer IDs this node is allowed to connect to.
package peers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Store is an in-memory set of known peer IDs, optionally loaded from a JSON
// file containing an array of peer ID strings.
type Store struct {
	path string
	ids  []peer.ID
	set  map[peer.ID]struct{}
}

// Load reads path. A missing file yields an empty store, not an error.
func Load(path string) (*Store, error) {
	s := &Store{path: path, set: make(map[peer.ID]struct{})}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read known peers %q: %w", path, err)
	}

	var raw []string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse known peers %q: %w", path, err)
	}

	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		id, err := peer.Decode(entry)
		if err != nil {
			return nil, fmt.Errorf("parse known peer %q in %s: %w", entry, path, err)
		}
		s.Add(id)
	}
	return s, nil
}

// Path returns the file this store was loaded from.
func (s *Store) Path() string {
	return s.path
}

// IDs returns the known peer IDs in insertion order.
func (s *Store) IDs() []peer.ID {
	out := make([]peer.ID, len(s.ids))
	copy(out, s.ids)
	return out
}

// Len returns the number of known peers.
func (s *Store) Len() int {
	return len(s.ids)
}

// Contains reports whether id is a known peer.
func (s *Store) Contains(id peer.ID) bool {
	_, ok := s.set[id]
	return ok
}

// Add inserts ids, ignoring duplicates.
func (s *Store) Add(ids ...peer.ID) {
	for _, id := range ids {
		if _, ok := s.set[id]; ok {
			continue
		}
		s.set[id] = struct{}{}
		s.ids = append(s.ids, id)
	}
}
