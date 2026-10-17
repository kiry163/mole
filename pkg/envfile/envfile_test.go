package envfile

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	data := []byte(`
# comment
PATH=/opt/homebrew/bin:/usr/bin
export TOKEN="a b#c"
EMPTY=
DUP=one
DUP=two
LITERAL_HASH=a#b
`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := map[string]string{
		"PATH":         "/opt/homebrew/bin:/usr/bin",
		"TOKEN":        "a b#c",
		"EMPTY":        "",
		"DUP":          "two",
		"LITERAL_HASH": "a#b",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseRejectsInvalidKeyAndMissingEquals(t *testing.T) {
	for _, tc := range []string{"BAD-KEY=x\n", "NOEQUALS\n"} {
		if _, err := Parse([]byte(tc)); err == nil {
			t.Fatalf("Parse(%q) expected error", tc)
		}
	}
}

func TestLoadAndApplySkipsBootstrapAndSetsProcessEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("MOLE_HOME=/ignored\nTEST_ENVFILE_VALUE=ok\nPATH=/bin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_ENVFILE_VALUE", "old")
	ignored, err := LoadAndApply(path)
	if err != nil {
		t.Fatalf("LoadAndApply() error = %v", err)
	}
	if !reflect.DeepEqual(ignored, []string{"MOLE_HOME"}) {
		t.Fatalf("ignored = %#v", ignored)
	}
	if got := os.Getenv("TEST_ENVFILE_VALUE"); got != "ok" {
		t.Fatalf("TEST_ENVFILE_VALUE = %q, want ok", got)
	}
	if got := os.Getenv("MOLE_HOME"); got == "/ignored" {
		t.Fatal("MOLE_HOME must not be overridden by .env")
	}
}

func TestLoadAndApplyMissingFileIsNoop(t *testing.T) {
	ignored, err := LoadAndApply(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil || len(ignored) != 0 {
		t.Fatalf("missing file = (%v, %v), want (nil, nil)", ignored, err)
	}
}

func TestUpdatePreservesCommentsAndUnknownLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	original := "# keep\nPATH=/old\nTOKEN=secret\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Update(path, map[string]string{"PATH": "/new path", "NEW": "x"}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, want := range []string{"# keep\n", "PATH='/new path'\n", "TOKEN=secret\n", "NEW=x\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated file missing %q:\n%s", want, got)
		}
	}
}

func TestReplaceManagedBlockReplacesOnlyBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	original := "# user\nPATH=/bin\n" + ManagedStart + "\nOLD=1\n" + ManagedEnd + "\nTAIL=keep\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceManagedBlock(path, ManagedStart, ManagedEnd, map[string]string{"NEW": "2"}); err != nil {
		t.Fatalf("ReplaceManagedBlock() error = %v", err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	if strings.Contains(got, "OLD=1") || !strings.Contains(got, "NEW=2") {
		t.Fatalf("managed block not replaced:\n%s", got)
	}
	for _, keep := range []string{"# user\n", "PATH=/bin\n", "TAIL=keep\n"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("lost user line %q:\n%s", keep, got)
		}
	}
}

func TestUpdateEnforces0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode check")
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Update(path, map[string]string{"A": "2"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}
