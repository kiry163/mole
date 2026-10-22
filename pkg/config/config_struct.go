package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/kiry163/mole/pkg/credential"
	"github.com/kiry163/mole/pkg/logger"
)

// FlexibleStringSlice is a []string that also accepts JSON numbers,
// so allow_from can contain both "123" and 123.
// It also supports parsing comma-separated strings from environment variables,
// including both English (,) and Chinese (，) commas.
type FlexibleStringSlice []string

func (f *FlexibleStringSlice) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*f = nil
		return nil
	}

	// Accept a single JSON string for convenience, e.g.:
	// "text": "Thinking..."
	var singleString string
	if err := json.Unmarshal(data, &singleString); err == nil {
		*f = FlexibleStringSlice{singleString}
		return nil
	}

	// Accept a single JSON number too, to keep symmetry with mixed allow_from
	// payloads that may contain numeric identifiers.
	var singleNumber float64
	if err := json.Unmarshal(data, &singleNumber); err == nil {
		*f = FlexibleStringSlice{fmt.Sprintf("%.0f", singleNumber)}
		return nil
	}

	// Try []string first
	var ss []string
	if err := json.Unmarshal(data, &ss); err == nil {
		*f = ss
		return nil
	}

	// Try []interface{} to handle mixed types
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	result := make([]string, 0, len(raw))
	for _, v := range raw {
		switch val := v.(type) {
		case string:
			result = append(result, val)
		case float64:
			result = append(result, fmt.Sprintf("%.0f", val))
		default:
			result = append(result, fmt.Sprintf("%v", val))
		}
	}
	*f = result
	return nil
}

// UnmarshalText implements encoding.TextUnmarshaler to support env variable parsing.
// It handles comma-separated values with both English (,) and Chinese (，) commas.
func (f *FlexibleStringSlice) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*f = nil
		return nil
	}

	s := string(text)
	// Replace Chinese comma with English comma, then split
	s = strings.ReplaceAll(s, "，", ",")
	parts := strings.Split(s, ",")

	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	*f = result
	return nil
}

const (
	notHere = `"[NOT_HERE]"`
)

// SecureStrings is a slice of SecureString
//
//nolint:recvcheck
type SecureStrings []*SecureString

// IsZero returns true if the SecureStrings is nil or empty.
func (s SecureStrings) IsZero() bool {
	return len(s) == 0
}

// Values returns the decrypted/resolved values
func (s *SecureStrings) Values() []string {
	if s == nil {
		return nil
	}
	keys := make([]string, len(*s))
	for i, k := range *s {
		keys[i] = k.String()
	}
	return unique(keys)
}

func SimpleSecureStrings(val ...string) SecureStrings {
	val = unique(val)
	vv := make(SecureStrings, len(val))
	for i, s := range val {
		vv[i] = NewSecureString(s)
	}
	return vv
}

// unique returns a new slice with duplicate elements removed.
func unique[T comparable](input []T) []T {
	m := make(map[T]struct{})
	var result []T
	for _, v := range input {
		if _, ok := m[v]; !ok {
			m[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

func (s SecureStrings) MarshalJSON() ([]byte, error) {
	out := make([]*SecureString, 0, len(s))
	for _, item := range s {
		if item != nil && !item.IsZero() {
			out = append(out, item)
		}
	}
	return json.Marshal(out)
}

func (s *SecureStrings) UnmarshalJSON(value []byte) error {
	if string(value) == notHere {
		return nil
	}
	var v []*SecureString
	err := json.Unmarshal(value, &v)
	if err != nil {
		return err
	}
	// Filter out elements where SecureString.UnmarshalJSON was a no-op
	// (e.g. "[NOT_HERE]" entries), keeping only actually populated values.
	filtered := make(SecureStrings, 0, len(v))
	for _, ss := range v {
		if ss == nil {
			continue
		}
		if ss.resolved != "" || ss.raw != "" {
			filtered = append(filtered, ss)
		}
	}
	if len(filtered) == 0 {
		*s = nil
	} else {
		*s = filtered
	}
	return nil
}

// SecureString the string value that can be decrypted or resolved
//
//nolint:recvcheck
type SecureString struct {
	resolved string // Decrypted/resolved value returned by String()
	raw      string // Persisted raw value (file:// or plaintext)
}

// IsZero returns true if the SecureString is empty.
func (s SecureString) IsZero() bool {
	return s.raw == "" && s.resolved == ""
}

func NewSecureString(value string) *SecureString {
	s := &SecureString{}
	if err := s.fromRaw(value); err != nil {
		logger.Warn(fmt.Sprintf("NewSecureString.fromRaw error: %s", err))
	}
	return s
}

func (s *SecureString) String() string {
	if s == nil {
		return ""
	}
	return s.resolved
}

func (s *SecureString) Set(value string) *SecureString {
	s.resolved = value
	s.raw = ""
	return s
}

// MarshalJSON emits the persisted reference (or plaintext when there is no
// reference) so config.json stores $VAR / plaintext values verbatim.
func (s SecureString) MarshalJSON() ([]byte, error) {
	value := s.raw
	if value == "" {
		value = s.resolved
	}
	return json.Marshal(value)
}

func (s *SecureString) UnmarshalJSON(value []byte) error {
	if string(value) == notHere {
		return nil
	}
	var v string
	if err := json.Unmarshal(value, &v); err != nil {
		return err
	}
	return s.fromRaw(v)
}

func (s *SecureString) fromRaw(v string) error {
	s.raw = v
	if strings.HasPrefix(v, "$$") {
		s.resolved = v[1:]
		return nil
	}
	if value, ok, err := lookupWholeEnvRef(v); err != nil {
		return err
	} else if ok {
		s.resolved = value
		return nil
	}
	resolved, err := resolveKey(v)
	if err != nil {
		return err
	}
	s.resolved = resolved
	return nil
}

var (
	secResolverMu sync.RWMutex
	secResolver   *credential.Resolver
)

func updateResolver(path string) {
	secResolverMu.Lock()
	defer secResolverMu.Unlock()
	secResolver = credential.NewResolver(path)
}

func resolveKey(v string) (string, error) {
	secResolverMu.RLock()
	resolver := secResolver
	secResolverMu.RUnlock()
	if resolver == nil {
		resolver = credential.NewResolver("")
	}
	if strings.HasPrefix(v, "enc://") {
		return "", fmt.Errorf("config: enc:// is no longer supported; use plaintext or $VAR")
	}
	if strings.HasPrefix(v, "file://") {
		decrypted, err := resolver.Resolve(v)
		if err != nil {
			logger.Errorf("Resolve error: %v", err)
			return "", err
		}
		return decrypted, nil
	}
	return v, nil
}

func (s *SecureString) UnmarshalText(text []byte) error {
	v := string(text)
	return s.fromRaw(v)
}

type SecureModelList []*ModelConfig
