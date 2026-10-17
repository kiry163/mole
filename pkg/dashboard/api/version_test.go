package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

// stubBuildInfo replaces the build metadata the binary carries and restores it
// afterwards.
func stubBuildInfo(t *testing.T, version, gitCommit, buildTime, goVersion string) {
	t.Helper()

	oldVersion, oldGitCommit := config.Version, config.GitCommit
	oldBuildTime, oldGoVersion := config.BuildTime, config.GoVersion
	t.Cleanup(func() {
		config.Version, config.GitCommit = oldVersion, oldGitCommit
		config.BuildTime, config.GoVersion = oldBuildTime, oldGoVersion
	})

	config.Version, config.GitCommit = version, gitCommit
	config.BuildTime, config.GoVersion = buildTime, goVersion
}

func TestGetVersionReportsInProcessBuildInfo(t *testing.T) {
	stubBuildInfo(t, "1.2.3", "abc1234", "2026-01-02T03:04:05Z", "go1.25.0")

	h := NewHandler("")
	rec := httptest.NewRecorder()
	h.handleGetVersion(rec, httptest.NewRequest(http.MethodGet, "/api/system/version", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got systemVersionResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Version != "1.2.3 (git: abc1234)" {
		t.Fatalf("version = %q, want %q", got.Version, "1.2.3 (git: abc1234)")
	}
	if got.GitCommit != "abc1234" {
		t.Fatalf("git_commit = %q, want %q", got.GitCommit, "abc1234")
	}
	if got.BuildTime != "2026-01-02T03:04:05Z" {
		t.Fatalf("build_time = %q, want %q", got.BuildTime, "2026-01-02T03:04:05Z")
	}
	if got.GoVersion != "go1.25.0" {
		t.Fatalf("go_version = %q, want %q", got.GoVersion, "go1.25.0")
	}
}

func TestGetVersionPrefersAttachedRuntimeVersion(t *testing.T) {
	stubBuildInfo(t, "1.2.3", "abc1234", "", "")

	h := NewHandler("")
	h.SetRuntime(&Runtime{Address: "127.0.0.1:18790", Version: "9.9.9"})

	rec := httptest.NewRecorder()
	h.handleGetVersion(rec, httptest.NewRequest(http.MethodGet, "/api/system/version", nil))

	var got systemVersionResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Version != "9.9.9" {
		t.Fatalf("version = %q, want %q", got.Version, "9.9.9")
	}
	if got.GitCommit != "abc1234" {
		t.Fatalf("git_commit = %q, want %q", got.GitCommit, "abc1234")
	}
}

func TestVersionRouteIsRegistered(t *testing.T) {
	h := NewHandler("")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/system/version", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
