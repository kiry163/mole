// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLogLines  = 200
	logFollowPoll    = 200 * time.Millisecond
	logTailChunkSize = 32 * 1024
)

// TailLogs writes the recent service log to w. On macOS it reads launchd's
// stdout/stderr files (falling back to gateway.log); on Linux it prefers the
// user journalctl unit and falls back to gateway.log; Windows reads
// gateway.log. With follow it keeps emitting appended output until ctx is
// cancelled.
func TailLogs(ctx context.Context, spec Spec, w io.Writer, lines int, follow bool) error {
	if lines <= 0 {
		lines = defaultLogLines
	}

	var (
		journalErr   error
		journalEmpty bool
	)
	if spec.goos() == "linux" && journalUnitPresent(spec) {
		if journalctl, err := exec.LookPath("journalctl"); err == nil {
			args := []string{"--user", "-u", spec.Name(), "-n", strconv.Itoa(lines)}
			if follow {
				args = append(args, "-f")
			}
			cmd := exec.CommandContext(ctx, journalctl, args...)
			if !follow {
				// Buffer the journal output: a successful but empty journal
				// must fall back to the file log without duplicating text.
				var out bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &out
				journalErr = cmd.Run()
				if ctx.Err() != nil {
					return nil
				}
				if journalErr == nil {
					if journalOutputEmpty(out.Bytes()) {
						journalEmpty = true
					} else {
						_, err := out.WriteTo(w)
						return err
					}
				}
			} else {
				cmd.Stdout = w
				cmd.Stderr = w
				journalErr = cmd.Run()
				if ctx.Err() != nil {
					return nil
				}
				if journalErr == nil {
					return nil
				}
			}
			// The unit may be unavailable (for example after an uninstall).
			// Prefer the file log next.
		}
	}

	paths := existingLogFiles(spec)
	if len(paths) == 0 {
		if journalErr != nil {
			return journalErr
		}
		if journalEmpty {
			return nil
		}
		return fmt.Errorf("no log file found for %s under %s", spec.Name(), filepath.Join(spec.HomePath, "logs"))
	}
	for _, path := range paths {
		if err := tailFile(path, lines, w); err != nil {
			return fmt.Errorf("tail %s: %w", path, err)
		}
	}
	if !follow {
		return nil
	}
	return followFiles(ctx, paths, w)
}

// journalUnitPresent reports whether the Linux unit file exists. Without a
// unit, journalctl cannot have entries for it (for example after uninstall).
func journalUnitPresent(spec Spec) bool {
	unit, err := spec.UnitPath()
	if err != nil || unit == "" {
		return true
	}
	_, err = os.Stat(unit)
	return err == nil
}

// journalOutputEmpty reports whether journalctl produced no entries.
func journalOutputEmpty(out []byte) bool {
	trimmed := strings.TrimSpace(string(out))
	return trimmed == "" || trimmed == "-- No entries --"
}

// logFiles returns the candidate log files in display order. gateway.log is
// the cross-platform fallback.
func logFiles(spec Spec) []string {
	logDir := filepath.Join(spec.HomePath, "logs")
	gateway := filepath.Join(logDir, "gateway.log")
	if spec.goos() != "darwin" {
		return []string{gateway}
	}
	return []string{
		filepath.Join(logDir, spec.Name()+".out.log"),
		filepath.Join(logDir, spec.Name()+".err.log"),
		gateway,
	}
}

// existingLogFiles keeps candidate files that exist. On macOS launchd's out/err
// files take precedence over gateway.log when either is present.
func existingLogFiles(spec Spec) []string {
	var paths []string
	for _, path := range logFiles(spec) {
		if _, err := os.Stat(path); err == nil {
			if spec.goos() == "darwin" && filepath.Base(path) == "gateway.log" && len(paths) > 0 {
				break
			}
			paths = append(paths, path)
		}
	}
	return paths
}

// tailFile writes the last lines of path to w. lines <= 0 copies the whole file.
func tailFile(path string, lines int, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	start, err := tailStart(f, info.Size(), lines)
	if err != nil {
		return err
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

// tailStart scans backwards for the offset that leaves at most lines lines.
func tailStart(f *os.File, size int64, lines int) (int64, error) {
	if lines <= 0 || size == 0 {
		return 0, nil
	}

	// A trailing newline terminates the last line; it does not start another.
	end := size
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, size-1); err != nil {
		return 0, err
	}
	if last[0] == '\n' {
		end--
	}

	remaining := lines
	buf := make([]byte, logTailChunkSize)
	for end > 0 {
		start := end - logTailChunkSize
		if start < 0 {
			start = 0
		}
		n := int(end - start)
		if _, err := f.ReadAt(buf[:n], start); err != nil && err != io.EOF {
			return 0, err
		}
		for i := n - 1; i >= 0; i-- {
			if buf[i] != '\n' {
				continue
			}
			remaining--
			if remaining == 0 {
				return start + int64(i) + 1, nil
			}
		}
		end = start
	}
	return 0, nil
}

type logCursor struct {
	path   string
	offset int64
}

func followFiles(ctx context.Context, paths []string, w io.Writer) error {
	cursors := make([]logCursor, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		cursors = append(cursors, logCursor{path: path, offset: info.Size()})
	}
	if len(cursors) == 0 {
		return fmt.Errorf("no log files to follow")
	}

	ticker := time.NewTicker(logFollowPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for i := range cursors {
				if err := readNewLog(&cursors[i], w); err != nil {
					return err
				}
			}
		}
	}
}

func readNewLog(cursor *logCursor, w io.Writer) error {
	f, err := os.Open(cursor.path)
	if err != nil {
		if os.IsNotExist(err) {
			cursor.offset = 0
			return nil
		}
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < cursor.offset {
		// The file was truncated or rotated.
		cursor.offset = 0
	}
	if _, err := f.Seek(cursor.offset, io.SeekStart); err != nil {
		return err
	}
	n, err := io.Copy(w, f)
	cursor.offset += n
	return err
}
