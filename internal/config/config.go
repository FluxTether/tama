// Package config resolves where tama keeps its local state (identity keys,
// database, ...) and loads environment configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultDataDirName is the data directory created next to the code.
	DefaultDataDirName = "data"

	// EnvFileName is the dotenv file loaded on startup.
	EnvFileName = ".env"

	// IdentityFileName stores the node's private key and peer ID.
	IdentityFileName = "identity.json"

	// KnownPeersFileName stores the list of peers this node may connect to.
	KnownPeersFileName = "peers.json"
)

// Config holds the resolved on-disk locations used by tama.
type Config struct {
	// Dir is the root directory where all tama data is stored.
	Dir string
}

// DefaultDir returns the default data directory: ./data, resolved against the
// current working directory. Keeping it next to the code makes the identity,
// database and UI assets easy to find and back up together.
func DefaultDir() (string, error) {
	return filepath.Abs(DefaultDataDirName)
}

// New returns a Config rooted at dir. If dir is empty, DefaultDir is used.
// The directory is created if it does not exist yet.
func New(dir string) (*Config, error) {
	if dir == "" {
		var err error
		dir, err = DefaultDir()
		if err != nil {
			return nil, fmt.Errorf("resolve default data dir: %w", err)
		}
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve data dir %q: %w", dir, err)
	}

	// 0700: the directory contains private key material.
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", abs, err)
	}

	return &Config{Dir: abs}, nil
}

// IdentityPath is the absolute path of the identity file.
func (c *Config) IdentityPath() string {
	return filepath.Join(c.Dir, IdentityFileName)
}

// KnownPeersPath is the absolute path of the known-peer list.
func (c *Config) KnownPeersPath() string {
	return filepath.Join(c.Dir, KnownPeersFileName)
}
