package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Environment variable names understood by tama.
const (
	EnvWebPort     = "WEB_PORT"
	EnvWSRelay     = "WS_RELAY"
	EnvLibp2pRelay = "LIBP2P_RELAY"
)

// DefaultWebPort is used when WEB_PORT is not set.
const DefaultWebPort = 8080

// Env holds runtime configuration loaded from the environment (.env plus the
// process environment, which takes precedence).
type Env struct {
	// WebPort is the HTTP port used by the web UI.
	WebPort int

	// WSRelay is the WebSocket relay base URL. Per-peer rooms are appended as
	// /<peer-id>. Empty disables internet peer lookup.
	WSRelay string

	// Libp2pRelay is the relay multiaddr, including the relay's peer ID, e.g.
	// /ip4/203.0.113.10/tcp/4001/p2p/12D3KooW...  Empty disables relay
	// connectivity and AutoRelay.
	Libp2pRelay string
}

// LoadEnv loads path as a dotenv file (a missing file is not an error) and
// reads tama's configuration from the resulting environment.
func LoadEnv(path string) (Env, error) {
	if path == "" {
		path = EnvFileName
	}

	if err := godotenv.Load(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Env{}, fmt.Errorf("load %q: %w", path, err)
	}

	env := Env{
		WebPort:     DefaultWebPort,
		WSRelay:     os.Getenv(EnvWSRelay),
		Libp2pRelay: os.Getenv(EnvLibp2pRelay),
	}

	if raw := os.Getenv(EnvWebPort); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return Env{}, fmt.Errorf("parse %s=%q: %w", EnvWebPort, raw, err)
		}
		if port < 1 || port > 65535 {
			return Env{}, fmt.Errorf("%s=%d is out of range 1-65535", EnvWebPort, port)
		}
		env.WebPort = port
	}

	return env, nil
}
