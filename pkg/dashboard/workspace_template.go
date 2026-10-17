package dashboard

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kiry163/mole/pkg/logger"
)

const (
	workspaceDirMode  fs.FileMode = 0o755
	workspaceFileMode fs.FileMode = 0o644
	workspaceExecMode fs.FileMode = 0o755
)

// expandWorkspaceTemplate writes the files under root in fsys into dir, creating
// only what is missing. Every file is created with O_EXCL rather than stat-ed
// first, so an existing entry - file, directory or symlink - is never truncated,
// rewritten or chmod-ed: the workspace belongs to its user from the moment it
// exists, and two processes racing on the same fresh workspace cannot destroy
// each other's work. Existing directories are reused. Directories are created
// 0755; files are created 0644, or 0755 for paths ending in .sh (the embed
// cannot carry the executable bit at all - see the Global Constraints - so the
// path decides).
//
// The walk stops at the first failure, which leaves a partially expanded tree.
// Only files this call itself created are ever removed - a failed write or close
// deletes its own new file so a later run retries it - and nothing pre-existing
// is overwritten, so a later run resumes the expansion instead of adopting a
// half-written file.
func expandWorkspaceTemplate(fsys fs.FS, root, dir string) (created, kept []string, err error) {
	prefix := root + "/"
	err = fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		// Embed paths are always slash-separated; convert only for the OS call.
		rel := filepath.FromSlash(strings.TrimPrefix(p, prefix))
		target := filepath.Join(dir, rel)

		data, readErr := fs.ReadFile(fsys, p)
		if readErr != nil {
			return fmt.Errorf("workspace template: read %s: %w", p, readErr)
		}
		if mkErr := os.MkdirAll(filepath.Dir(target), workspaceDirMode); mkErr != nil {
			return fmt.Errorf("workspace template: mkdir %s: %w", filepath.Dir(target), mkErr)
		}

		// Go's embed reports 0444 for every embedded file, so executability is a
		// property of the template convention rather than of the FS: shell scripts
		// shipped in the template are meant to be run.
		mode := workspaceFileMode
		if strings.HasSuffix(rel, ".sh") {
			mode = workspaceExecMode
		}
		// O_EXCL, not Stat-then-Write: two processes starting on the same fresh
		// workspace must not be able to truncate a file the other just created.
		// An existing entry (file, directory, or symlink) is kept untouched.
		f, openErr := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(openErr, fs.ErrExist) {
			kept = append(kept, rel)
			return nil
		}
		if openErr != nil {
			return fmt.Errorf("workspace template: create %s: %w", target, openErr)
		}
		if _, writeErr := f.Write(data); writeErr != nil {
			_ = f.Close()
			// The file was created by this call under O_EXCL, so it cannot be a
			// pre-existing user file: removing it keeps a failed write from
			// leaving a 0-byte entry that every later start would count as kept.
			_ = os.Remove(target)
			return fmt.Errorf("workspace template: write %s: %w", target, writeErr)
		}
		if closeErr := f.Close(); closeErr != nil {
			// Same reasoning as the write branch: the bytes on disk are not known
			// to be complete, and a kept half-written file would never be retried.
			_ = os.Remove(target)
			return fmt.Errorf("workspace template: close %s: %w", target, closeErr)
		}
		created = append(created, rel)
		return nil
	})
	return created, kept, err
}

// ensureWorkspaceTemplate is the startup entry point. A failure is logged and
// otherwise ignored: a template that cannot be written must not keep the console
// - and therefore the setup page - from starting, and the next start retries it.
// A steady-state start creates nothing and logs nothing.
func ensureWorkspaceTemplate(dir string) {
	created, kept, err := expandWorkspaceTemplate(workspaceTemplate, workspaceTemplateRoot, dir)
	if err != nil {
		// The wrapped errors already carry the "workspace template:" prefix.
		logger.ErrorC("web", err.Error())
		return
	}
	if len(created) > 0 {
		logger.InfoC("web", fmt.Sprintf("workspace template: initialised %d file(s) in %s (%d kept)",
			len(created), dir, len(kept)))
	}
}
