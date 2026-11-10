package config

import (
	"os"
	"path/filepath"
	"testing"
)

// unsetEnv removes the tama environment variables for the duration of the test
// and restores their previous values afterwards.
func unsetEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{EnvWebPort, EnvWSRelay, EnvLibp2pRelay} {
		key := key
		old, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if existed {
				os.Setenv(key, old)
			} else {
				os.Unsetenv(key)
			}
		})
	}
}

func TestLoadEnvDefaults(t *testing.T) {
	unsetEnv(t)

	env, err := LoadEnv(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if env.WebPort != DefaultWebPort {
		t.Fatalf("WebPort = %d, want %d", env.WebPort, DefaultWebPort)
	}
	if env.WSRelay != "" || env.Libp2pRelay != "" {
		t.Fatalf("relays should default to empty, got %q and %q", env.WSRelay, env.Libp2pRelay)
	}
}

func TestLoadEnvFromFile(t *testing.T) {
	unsetEnv(t)

	path := filepath.Join(t.TempDir(), ".env")
	content := "WEB_PORT=9090\nWS_RELAY=ws://relay:9000\nLIBP2P_RELAY=/ip4/10.0.0.1/tcp/4001\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	env, err := LoadEnv(path)
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if env.WebPort != 9090 {
		t.Fatalf("WebPort = %d, want 9090", env.WebPort)
	}
	if env.WSRelay != "ws://relay:9000" {
		t.Fatalf("WSRelay = %q", env.WSRelay)
	}
	if env.Libp2pRelay != "/ip4/10.0.0.1/tcp/4001" {
		t.Fatalf("Libp2pRelay = %q", env.Libp2pRelay)
	}
}

func TestLoadEnvRejectsInvalidWebPort(t *testing.T) {
	unsetEnv(t)
	t.Setenv(EnvWebPort, "70000")

	if _, err := LoadEnv(filepath.Join(t.TempDir(), "missing.env")); err == nil {
		t.Fatal("expected an error for an out-of-range WEB_PORT")
	}
}
