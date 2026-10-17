package dashboard

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func mapFS() fstest.MapFS {
	return fstest.MapFS{
		"workspace/AGENT.md":             &fstest.MapFile{Data: []byte("agent\n")},
		"workspace/skills/tmux/SKILL.md": &fstest.MapFile{Data: []byte("tmux\n")},
		// No Mode here on purpose: the expansion decides executability from the
		// path suffix, because embed (and any FS-shaped template source) cannot
		// carry the bit.
		"workspace/skills/tmux/scripts/wait-for-text.sh": &fstest.MapFile{Data: []byte("#!/bin/sh\n")},
	}
}

func TestExpandWorkspaceTemplateCreatesMissingFiles(t *testing.T) {
	dir := t.TempDir()

	created, kept, err := expandWorkspaceTemplate(mapFS(), "workspace", dir)
	if err != nil {
		t.Fatalf("expandWorkspaceTemplate() error = %v", err)
	}
	if len(kept) != 0 {
		t.Fatalf("kept = %v, want none on a fresh directory", kept)
	}
	if len(created) != 3 {
		t.Fatalf("created = %v, want 3 entries", created)
	}

	data, err := os.ReadFile(filepath.Join(dir, "AGENT.md"))
	if err != nil || string(data) != "agent\n" {
		t.Fatalf("AGENT.md = %q, err = %v", data, err)
	}

	// The directories the walk makes are 0755 like the files' 0644: a workspace
	// nobody else can enter breaks the console's own read of it.
	din, err := os.Stat(filepath.Join(dir, "skills/tmux"))
	if err != nil {
		t.Fatalf("stat skills/tmux: %v", err)
	}
	if din.Mode().Perm() != 0o755 {
		t.Fatalf("skills/tmux mode = %v, want 0755", din.Mode().Perm())
	}

	// Executability follows the template's .sh convention; everything else is 0644.
	info, err := os.Stat(filepath.Join(dir, "skills/tmux/scripts/wait-for-text.sh"))
	if err != nil {
		t.Fatalf("stat script: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("script mode = %v, want 0755", info.Mode().Perm())
	}
	info, err = os.Stat(filepath.Join(dir, "AGENT.md"))
	if err != nil {
		t.Fatalf("stat AGENT.md: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("AGENT.md mode = %v, want 0644", info.Mode().Perm())
	}
}

// The workspace belongs to its user the moment it exists: an existing file is
// never read, truncated or rewritten, whatever its content or mode.
func TestExpandWorkspaceTemplateNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "skills/tmux/scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := []byte("my own agent\n")
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), custom, 0o600); err != nil {
		t.Fatal(err)
	}
	readOnly := []byte("#!/bin/sh\necho mine\n")
	if err := os.WriteFile(filepath.Join(dir, "skills/tmux/scripts/wait-for-text.sh"), readOnly, 0o444); err != nil {
		t.Fatal(err)
	}

	created, kept, err := expandWorkspaceTemplate(mapFS(), "workspace", dir)
	if err != nil {
		t.Fatalf("expandWorkspaceTemplate() error = %v", err)
	}
	if len(created) != 1 || created[0] != "skills/tmux/SKILL.md" {
		t.Fatalf("created = %v, want only the missing skill", created)
	}
	if len(kept) != 2 {
		t.Fatalf("kept = %v, want the two existing files", kept)
	}

	got, err := os.ReadFile(filepath.Join(dir, "AGENT.md"))
	if err != nil || string(got) != string(custom) {
		t.Fatalf("AGENT.md = %q, err = %v; existing content must survive", got, err)
	}
	got, err = os.ReadFile(filepath.Join(dir, "skills/tmux/scripts/wait-for-text.sh"))
	if err != nil || string(got) != string(readOnly) {
		t.Fatalf("script = %q, err = %v; existing content must survive", got, err)
	}
	info, err := os.Stat(filepath.Join(dir, "AGENT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("AGENT.md mode = %v, want the existing 0600 preserved", info.Mode().Perm())
	}
}

// A second expansion of an already-initialised workspace writes nothing.
func TestExpandWorkspaceTemplateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := expandWorkspaceTemplate(mapFS(), "workspace", dir); err != nil {
		t.Fatalf("first expand: %v", err)
	}

	created, kept, err := expandWorkspaceTemplate(mapFS(), "workspace", dir)
	if err != nil {
		t.Fatalf("second expand: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("second expand created %v, want nothing", created)
	}
	if len(kept) != 3 {
		t.Fatalf("second expand kept %v, want all three files", kept)
	}
}

// A path blocked by a plain file cannot be turned into a directory: the walk
// must report it instead of skipping silently, the files written before the
// failure must survive, and a later run (once the blocker is gone) must finish
// the job - the expansion is resumable because nothing pre-existing is ever
// overwritten and only its own half-written files are removed.
func TestExpandWorkspaceTemplateReportsBlockedPathsAndResumes(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "skills")
	if err := os.WriteFile(blocker, []byte("not a dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	created, _, err := expandWorkspaceTemplate(mapFS(), "workspace", dir)
	if err == nil {
		t.Fatal("expandWorkspaceTemplate() = nil error, want a mkdir failure")
	}
	if len(created) != 1 || created[0] != "AGENT.md" {
		t.Fatalf("created = %v, want AGENT.md to have been written before the failure", created)
	}
	if _, statErr := os.Stat(blocker); statErr != nil {
		t.Fatalf("the blocker must survive a failed expansion: %v", statErr)
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	resumedCreated, resumedKept, err := expandWorkspaceTemplate(mapFS(), "workspace", dir)
	if err != nil {
		t.Fatalf("resumed expand: %v", err)
	}
	if len(resumedCreated) != 2 {
		t.Fatalf("resumed created = %v, want the two skill files", resumedCreated)
	}
	if len(resumedKept) != 1 || resumedKept[0] != "AGENT.md" {
		t.Fatalf("resumed kept = %v, want AGENT.md", resumedKept)
	}
}

// The mkdir failure needs a blocker that gets past the walk as far as MkdirAll.
// A plain file at dir/skills (covered above) is turned away by its own EEXIST,
// but a *dangling symlink* there looks absent to Stat and only stops the walk at
// MkdirAll - so this is the shape that pins that failure to its own branch
// instead of letting the ENOENT of the create that follows stand in for it.
func TestExpandWorkspaceTemplateReportsMkdirFailure(t *testing.T) {
	dir := t.TempDir()
	dangling := filepath.Join(dir, "skills")
	if err := os.Symlink(filepath.Join(dir, "no-such-target"), dangling); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	created, _, err := expandWorkspaceTemplate(mapFS(), "workspace", dir)
	if err == nil {
		t.Fatal("expandWorkspaceTemplate() = nil error, want a mkdir failure")
	}
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || pathErr.Op != "mkdir" {
		t.Fatalf("error = %v, want the failure to come from MkdirAll", err)
	}
	if len(created) != 1 || created[0] != "AGENT.md" {
		t.Fatalf("created = %v, want AGENT.md to have been written before the failure", created)
	}
	if _, linkErr := os.Lstat(dangling); linkErr != nil {
		t.Fatalf("the dangling symlink must survive a failed expansion: %v", linkErr)
	}
}
