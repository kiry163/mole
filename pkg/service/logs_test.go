// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTailFileLastLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		lines   int
		want    string
	}{
		{"trailing newline", "one\ntwo\nthree\nfour\n", 2, "three\nfour\n"},
		{"no trailing newline", "one\ntwo\nthree", 2, "two\nthree"},
		{"single line", "one\ntwo\nthree\n", 1, "three\n"},
		{"more lines than content", "one\ntwo\n", 10, "one\ntwo\n"},
		{"all lines", "one\ntwo\n", 0, "one\ntwo\n"},
		{"empty", "", 2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gateway.log")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := tailFile(path, tt.lines, &buf); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tt.want {
				t.Fatalf("tailFile(%d) = %q, want %q", tt.lines, got, tt.want)
			}
		})
	}
}

func TestTailLogsGatewayFallback(t *testing.T) {
	home := t.TempDir()
	logPath := filepath.Join(home, "logs", "gateway.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, goos := range []string{"windows", "darwin"} {
		var buf bytes.Buffer
		spec := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), GOOS: goos}
		if err := TailLogs(context.Background(), spec, &buf, 2, false); err != nil {
			t.Fatalf("%s TailLogs() error = %v", goos, err)
		}
		if got := buf.String(); got != "two\nthree\n" {
			t.Fatalf("%s TailLogs() = %q, want %q", goos, got, "two\nthree\n")
		}
	}
}

func TestTailLogsDarwinLaunchdFiles(t *testing.T) {
	home := t.TempDir()
	spec := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), GOOS: "darwin"}
	logDir := filepath.Join(home, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(logDir, spec.Name()+".out.log")
	errPath := filepath.Join(logDir, spec.Name()+".err.log")
	if err := os.WriteFile(outPath, []byte("out-old\nout-new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(errPath, []byte("err-old\nerr-new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// gateway.log must not be used when launchd logs exist.
	if err := os.WriteFile(filepath.Join(logDir, "gateway.log"), []byte("gateway\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := TailLogs(context.Background(), spec, &buf, 1, false); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "out-new\n") || !strings.Contains(got, "err-new\n") {
		t.Fatalf("TailLogs() = %q, want out and err last lines", got)
	}
	if strings.Contains(got, "out-old") || strings.Contains(got, "err-old") || strings.Contains(got, "gateway") {
		t.Fatalf("TailLogs() = %q, want only last launchd lines", got)
	}
}

func TestTailLogsLinuxJournalctlFallback(t *testing.T) {
	if _, err := exec.LookPath("journalctl"); err == nil {
		t.Skip("journalctl available; fallback cannot be exercised")
	}
	home := t.TempDir()
	logPath := filepath.Join(home, "logs", "gateway.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("linux-old\nlinux-new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), GOOS: "linux"}
	var buf bytes.Buffer
	if err := TailLogs(context.Background(), spec, &buf, 1, false); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "linux-new\n" {
		t.Fatalf("TailLogs() = %q, want %q", got, "linux-new\n")
	}
}

func TestTailLogsFollowCancellation(t *testing.T) {
	home := t.TempDir()
	logPath := filepath.Join(home, "logs", "gateway.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var buf syncBuffer
	done := make(chan error, 1)
	spec := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), GOOS: "windows"}
	go func() { done <- TailLogs(ctx, spec, &buf, 1, true) }()

	time.Sleep(150 * time.Millisecond)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("new\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(buf.String(), "new\n") {
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("follow did not emit appended line, output = %q", buf.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("TailLogs() error after cancel = %v", err)
	}
}

func TestTailLogsMissingFile(t *testing.T) {
	home := t.TempDir()
	spec := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), GOOS: "windows"}
	var buf bytes.Buffer
	if err := TailLogs(context.Background(), spec, &buf, 10, false); err == nil {
		t.Fatal("TailLogs() error = nil for missing log file")
	}
}

func TestTailLogsLinuxJournalctlFailureFallsBack(t *testing.T) {
	spec, _ := stubLinuxLogs(t, "fallback-old\nfallback-new\n")
	stubJournalctl(t, spec, "#!/bin/sh\necho journal unavailable >&2\nexit 1\n", true)

	var buf bytes.Buffer
	if err := TailLogs(context.Background(), spec, &buf, 1, false); err != nil {
		t.Fatalf("TailLogs() error = %v, want gateway.log fallback", err)
	}
	if got := buf.String(); !strings.Contains(got, "fallback-new\n") {
		t.Fatalf("TailLogs() = %q, want fallback log content", got)
	}
}

func TestTailLogsLinuxJournalctlEmptyFallsBack(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{"no entries", "#!/bin/sh\necho '-- No entries --'\n"},
		{"empty output", "#!/bin/sh\nexit 0\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, logPath := stubLinuxLogs(t, "fallback-old\nfallback-new\n")
			stubJournalctl(t, spec, tt.script, true)

			var buf bytes.Buffer
			if err := TailLogs(context.Background(), spec, &buf, 1, false); err != nil {
				t.Fatalf("TailLogs() error = %v, want gateway.log fallback", err)
			}
			if got := buf.String(); got != "fallback-new\n" {
				t.Fatalf("TailLogs() = %q, want only fallback log content", got)
			}
			if strings.Contains(buf.String(), "No entries") {
				t.Fatalf("TailLogs() = %q, must not duplicate journal output", buf.String())
			}
			if _, err := os.Stat(logPath); err != nil {
				t.Fatalf("fallback log missing: %v", err)
			}
		})
	}
}

func TestTailLogsLinuxSkipsJournalWhenUnitMissing(t *testing.T) {
	spec, _ := stubLinuxLogs(t, "fallback-new\n")
	stubJournalctl(t, spec, "#!/bin/sh\necho journal-only\n", false)

	var buf bytes.Buffer
	if err := TailLogs(context.Background(), spec, &buf, 1, false); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "fallback-new\n" {
		t.Fatalf("TailLogs() = %q, want file log when unit is absent", got)
	}
}

// stubLinuxLogs creates a gateway.log and returns the Linux spec for it.
func stubLinuxLogs(t *testing.T, content string) (Spec, string) {
	t.Helper()
	home := t.TempDir()
	logPath := filepath.Join(home, "logs", "gateway.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), GOOS: "linux"}
	return spec, logPath
}

// stubJournalctl puts a shell stub on PATH and optionally installs the unit
// file TailLogs checks before invoking journalctl.
func stubJournalctl(t *testing.T, spec Spec, script string, installUnit bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub journalctl is not executable on Windows")
	}
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	if installUnit {
		unit, err := spec.UnitPath()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(unit, []byte("unit"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	binDir := t.TempDir()
	path := filepath.Join(binDir, "journalctl")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
}

// syncBuffer serializes writes so a follow goroutine can be observed safely.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
