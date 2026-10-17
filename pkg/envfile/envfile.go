// Package envfile reads and writes the instance-scoped .env file.
//
// The file is a flat list of KEY=VALUE variables. It is not a config file:
// values are never expanded, never prefixed, and never mapped to config fields.
package envfile

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/kiry163/mole/pkg/fileutil"
)

const (
	ManagedStart = "# >>> mole managed secrets >>>"
	ManagedEnd   = "# <<< mole managed secrets <<<"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Parse parses KEY=VALUE lines. Only whole-line # comments are supported;
// a # inside a value is literal. Quotes are stripped, not unescaped.
func Parse(data []byte) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(data))
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf(".env line %d: missing '='", lineNo)
		}
		key = strings.TrimSpace(key)
		if !keyPattern.MatchString(key) {
			return nil, fmt.Errorf(".env line %d: invalid key %q", lineNo, key)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		out[key] = value
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// LoadAndApply parses path and sets every variable in the process environment.
// MOLE_HOME and MOLE_CONFIG are bootstrap variables and are returned as ignored.
// A missing file is a no-op.
func LoadAndApply(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	values, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ignored := make([]string, 0, 2)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "MOLE_HOME" || key == "MOLE_CONFIG" {
			ignored = append(ignored, key)
			continue
		}
		if err := os.Setenv(key, values[key]); err != nil {
			return nil, fmt.Errorf("set %s: %w", key, err)
		}
	}
	return ignored, nil
}

// Update rewrites the keys in updates while preserving comments and unknown
// lines. New keys are appended. The file is written atomically with mode 0600.
func Update(path string, updates map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := splitLines(data)
	seen := make(map[string]bool, len(updates))
	for i, line := range lines {
		key, ok := lineKey(line)
		if !ok {
			continue
		}
		value, exists := updates[key]
		if !exists {
			continue
		}
		lines[i] = key + "=" + quoteValue(value)
		seen[key] = true
	}
	for _, key := range sortedKeys(updates) {
		if !seen[key] {
			lines = append(lines, key+"="+quoteValue(updates[key]))
		}
	}
	return writeAtomic(path, []byte(strings.Join(lines, "\n")+"\n"))
}

// ReplaceManagedBlock removes the block between start and end (inclusive) and
// appends a fresh block at the end. An empty values map removes the block.
func ReplaceManagedBlock(path, start, end string, values map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := splitLines(data)
	kept := make([]string, 0, len(lines))
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == start:
			inBlock = true
		case trimmed == end:
			inBlock = false
		case inBlock:
			// drop old managed lines
		default:
			kept = append(kept, line)
		}
	}
	if len(values) > 0 {
		kept = append(kept, start)
		for _, key := range sortedKeys(values) {
			kept = append(kept, key+"="+quoteValue(values[key]))
		}
		kept = append(kept, end)
	}
	return writeAtomic(path, []byte(strings.Join(kept, "\n")+"\n"))
}

func splitLines(data []byte) []string {
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func lineKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	trimmed = strings.TrimPrefix(trimmed, "export ")
	key, _, ok := strings.Cut(trimmed, "=")
	if !ok {
		return "", false
	}
	key = strings.TrimSpace(key)
	return key, keyPattern.MatchString(key)
}

func quoteValue(value string) string {
	if value == "" || !strings.ContainsAny(value, " \t#\"'") {
		return value
	}
	if !strings.Contains(value, "'") {
		return "'" + value + "'"
	}
	if !strings.Contains(value, "\"") {
		return `"` + value + `"`
	}
	return value
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeAtomic(path string, data []byte) error {
	return fileutil.WriteFileAtomic(path, data, 0o600)
}
