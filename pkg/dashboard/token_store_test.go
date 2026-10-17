package dashboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/dashboard/api"
)

func newTokenStoreTestPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"version": 3}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestTokenStore_EmptyConfigIsUninitialized(t *testing.T) {
	store := newTokenStore(newTokenStoreTestPath(t))

	initialized, err := store.IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("IsInitialized() error = %v", err)
	}
	if initialized {
		t.Fatal("a config without dashboard.token must be uninitialized")
	}
}

func TestTokenStore_SetTokenPersistsToEnv(t *testing.T) {
	path := newTokenStoreTestPath(t)
	store := newTokenStore(path)

	if err := store.SetToken(context.Background(), "smoke-token"); err != nil {
		t.Fatalf("SetToken() error = %v", err)
	}

	initialized, err := store.IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("IsInitialized() error = %v", err)
	}
	if !initialized {
		t.Fatal("stored token should count as initialized")
	}

	rawConfig, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(rawConfig), `"$DASHBOARD_TOKEN"`) {
		t.Fatalf("config should reference $DASHBOARD_TOKEN, got:\n%s", rawConfig)
	}
	if strings.Contains(string(rawConfig), "smoke-token") {
		t.Fatalf("config leaked the token:\n%s", rawConfig)
	}

	rawEnv, err := os.ReadFile(config.EnvFilePath(path))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(rawEnv), "DASHBOARD_TOKEN=smoke-token") {
		t.Fatalf(".env should hold the token, got:\n%s", rawEnv)
	}
}

func TestTokenStore_VerifyTokenUsesConstantTimeCompare(t *testing.T) {
	store := newTokenStore(newTokenStoreTestPath(t))
	if err := store.SetToken(context.Background(), "right-token"); err != nil {
		t.Fatalf("SetToken() error = %v", err)
	}

	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{"correct", "right-token", true},
		{"wrong same length", "wrong-token", false},
		{"wrong shorter", "right", false},
		{"wrong longer", "right-token-plus-more", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.VerifyToken(context.Background(), tt.token)
			if err != nil {
				t.Fatalf("VerifyToken() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("VerifyToken(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

func TestTokenStore_BlankTokenIsNotInitialized(t *testing.T) {
	path := newTokenStoreTestPath(t)
	if err := os.WriteFile(path,
		[]byte(`{"version": 3, "dashboard": {"token": "   "}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	initialized, err := newTokenStore(path).IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("IsInitialized() error = %v", err)
	}
	if initialized {
		t.Fatal("a whitespace-only token must not count as initialized")
	}
}

func TestTokenStore_MaskedPlaceholderIsNotAToken(t *testing.T) {
	path := newTokenStoreTestPath(t)
	// config.json carries the JSON mask SecureString writes instead of the value.
	if err := os.WriteFile(path, []byte(`{"version": 3, "dashboard": {"token": "[NOT_HERE]"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	store := newTokenStore(path)
	initialized, err := store.IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("IsInitialized() error = %v", err)
	}
	if initialized {
		t.Fatal("the JSON mask must not be treated as a usable token")
	}
	// The mask is public knowledge (it is what JSON views hide secrets behind),
	// so accepting it as the token would hand the console to anyone who guesses
	// that literal string.
	ok, err := store.VerifyToken(context.Background(), "[NOT_HERE]")
	if err != nil {
		t.Fatalf("VerifyToken() error = %v", err)
	}
	if ok {
		t.Fatal("the JSON mask must never authenticate")
	}
}

func TestTokenStore_RejectsTheJSONMask(t *testing.T) {
	// Accepting it would report success to the caller and leave the console
	// unconfigured, because reads treat the mask as "no token set".
	path := newTokenStoreTestPath(t)
	store := newTokenStore(path)

	err := store.SetToken(context.Background(), "[NOT_HERE]")
	if err == nil {
		t.Fatal("SetToken() must reject the JSON mask")
	}
	if !errors.Is(err, api.ErrInvalidToken) {
		t.Fatalf("SetToken() error = %v, want it to wrap api.ErrInvalidToken so the API answers 400", err)
	}
	initialized, err := store.IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("IsInitialized() error = %v", err)
	}
	if initialized {
		t.Fatal("a rejected token must not be stored")
	}
}
