package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/kiry163/mole/pkg/envfile"
)

var envNameCleaner = regexp.MustCompile(`[^A-Z0-9]+`)

// EnvFilePath returns the instance .env path next to config.json.
func EnvFilePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), ".env")
}

// RedactSecrets empties every plaintext secret reachable from c, so the config
// can be marshaled to JSON without exposing credentials. $VAR references and
// file:// values stay intact: they name a secret, they do not reveal one.
func (c *Config) RedactSecrets() {
	if c == nil {
		return
	}
	redact := func(s *SecureString, _ []string, _ int) error {
		redactSecureString(s)
		return nil
	}
	if err := walkSecureStrings(reflect.ValueOf(c).Elem(), nil, redact); err != nil {
		return
	}
	// walkSecureStrings decodes channel settings through GetDecoded; walk the
	// stored extend value as well because it is what Channel.MarshalJSON emits.
	for _, ch := range c.Channels {
		if ch == nil || ch.extend == nil {
			continue
		}
		if err := walkSecureStrings(reflect.ValueOf(ch.extend), nil, redact); err != nil {
			return
		}
	}
}

// redactSecureString clears a plaintext value. A reference is kept because
// MarshalJSON emits the raw form, and the reference is not the secret.
func redactSecureString(s *SecureString) {
	if s == nil {
		return
	}
	if strings.HasPrefix(s.raw, "file://") || wholeEnvRefPattern.MatchString(s.raw) {
		return
	}
	s.raw = ""
	s.resolved = ""
}

// promoteSecrets rewrites every plaintext SecureString to a $VAR reference and
// writes the values into the managed block of .env. file:// values stay inline;
// enc:// is rejected.
func promoteSecrets(configPath string, cfg *Config) error {
	values := make(map[string]string)
	envPath := EnvFilePath(configPath)
	existing := map[string]string{}
	if data, err := os.ReadFile(envPath); err == nil {
		if parsed, parseErr := envfile.Parse(data); parseErr == nil {
			existing = parsed
		}
	}
	err := walkSecureStrings(reflect.ValueOf(cfg).Elem(), nil, func(s *SecureString, path []string, index int) error {
		if s == nil || (s.raw == "" && s.resolved == "") {
			return nil
		}
		if strings.HasPrefix(s.raw, "file://") {
			return nil
		}
		if strings.HasPrefix(s.raw, "enc://") {
			return fmt.Errorf("config: enc:// is no longer supported; use plaintext or $VAR")
		}
		if wholeEnvRefPattern.MatchString(s.raw) {
			match := wholeEnvRefPattern.FindStringSubmatch(s.raw)
			name := match[1]
			if name == "" {
				name = match[2]
			}
			// Preserve a value edited directly in .env; only fall back to the
			// process snapshot when the file has no entry.
			if value, ok := existing[name]; ok {
				values[name] = value
			} else if s.resolved != "" {
				values[name] = s.resolved
			} else {
				return fmt.Errorf("config: secret reference %q has no resolved value", s.raw)
			}
			return nil
		}
		value := s.resolved
		if value == "" {
			value = s.raw
		}
		name := secretEnvName(cfg, path, index)
		s.raw = "$" + name
		values[name] = value
		return nil
	})
	if err != nil {
		return err
	}
	if len(values) == 0 {
		if _, statErr := os.Stat(envPath); os.IsNotExist(statErr) {
			return nil
		}
	}
	if err := envfile.ReplaceManagedBlock(envPath, envfile.ManagedStart, envfile.ManagedEnd, values); err != nil {
		return err
	}
	// Make newly-saved secrets resolvable in the running process for the
	// current request. A full restart is still the supported way to apply
	// startup-time variables.
	for name, value := range values {
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("set %s: %w", name, err)
		}
	}
	return nil
}

var (
	secureStringType  = reflect.TypeOf(SecureString{})
	secureStringsType = reflect.TypeOf(SecureStrings{})
	channelType       = reflect.TypeOf(Channel{})
)

// walkSecureStrings visits every SecureString reachable from v. Struct fields
// are addressed with their Go field names; slice entries with their index;
// channel settings are decoded first so their SecureStrings are visible.
func walkSecureStrings(v reflect.Value, path []string, fn func(*SecureString, []string, int) error) error {
	if !v.IsValid() {
		return nil
	}
	if v.Type() == secureStringType {
		if !v.CanAddr() {
			return nil
		}
		return fn(v.Addr().Interface().(*SecureString), path, -1)
	}
	if v.Type() == secureStringsType {
		for i := range v.Len() {
			item := v.Index(i)
			if item.IsNil() {
				continue
			}
			if err := fn(item.Interface().(*SecureString), path, i); err != nil {
				return err
			}
		}
		return nil
	}
	if v.Type() == channelType || (v.Kind() == reflect.Pointer && v.Type().Elem() == channelType) {
		var ch *Channel
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return nil
			}
			ch = v.Interface().(*Channel)
		} else {
			if !v.CanAddr() {
				return nil
			}
			ch = v.Addr().Interface().(*Channel)
		}
		decoded, err := ch.GetDecoded()
		if err != nil || decoded == nil {
			return err
		}
		return walkSecureStrings(reflect.ValueOf(decoded), append(path, "settings"), fn)
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return walkSecureStrings(v.Elem(), path, fn)
	case reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return walkSecureStrings(v.Elem(), path, fn)
	case reflect.Struct:
		if !v.CanAddr() {
			return nil
		}
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			if err := walkSecureStrings(v.Field(i), append(path, field.Name), fn); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if err := walkSecureStrings(v.Index(i), append(path, strconv.Itoa(i)), fn); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if key.Kind() != reflect.String {
				continue
			}
			if err := walkSecureStrings(v.MapIndex(key), append(path, key.String()), fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func secretEnvName(cfg *Config, path []string, index int) string {
	if len(path) >= 2 && path[0] == "Dashboard" && path[1] == "Token" {
		return "DASHBOARD_TOKEN"
	}
	if len(path) >= 3 && path[0] == "Channels" {
		field := path[len(path)-1]
		return cleanEnvName(path[1] + "_" + field)
	}
	if len(path) >= 5 && path[0] == "Tools" && path[1] == "Skills" && path[2] == "Registries" {
		i, _ := strconv.Atoi(path[3])
		registry := "SKILLS_REGISTRY"
		if i >= 0 && i < len(cfg.Tools.Skills.Registries) && cfg.Tools.Skills.Registries[i] != nil {
			registry = cfg.Tools.Skills.Registries[i].Name
		}
		switch strings.ToLower(registry) {
		case "github":
			return "GITHUB_TOKEN"
		case "clawhub":
			return "CLAWHUB_TOKEN"
		default:
			return cleanEnvName(registry) + "_AUTH_TOKEN"
		}
	}
	if len(path) >= 3 && path[0] == "ModelList" && path[2] == "APIKeys" {
		i, _ := strconv.Atoi(path[1])
		name := "MODEL"
		dup := 0
		if i >= 0 && i < len(cfg.ModelList) && cfg.ModelList[i] != nil {
			model := cfg.ModelList[i]
			name = cleanEnvName(model.ModelName)
			for j := range i {
				if cfg.ModelList[j] != nil && cfg.ModelList[j].ModelName == model.ModelName {
					dup++
				}
			}
		}
		if dup > 0 {
			// "__DUP" cannot be produced by cleanEnvName, so a duplicate of
			// "gpt-4o" never collides with a distinct model "gpt-4o-2".
			name += "__DUP" + strconv.Itoa(dup+1)
		}
		name += "_API_KEY"
		if index > 0 {
			name += "_" + strconv.Itoa(index+1)
		}
		return name
	}
	if len(path) >= 4 && path[0] == "Tools" && path[1] == "Web" && path[2] == "Tavily" && path[3] == "APIKeys" {
		name := "TAVILY_API_KEY"
		if index > 0 {
			name += "_" + strconv.Itoa(index+1)
		}
		return name
	}
	base := cleanEnvName(strings.Join(path, "_"))
	if index > 0 {
		base += "_" + strconv.Itoa(index+1)
	}
	return base
}

func cleanEnvName(value string) string {
	value = strings.ToUpper(value)
	value = envNameCleaner.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "SECRET"
	}
	if value[0] >= '0' && value[0] <= '9' {
		value = "SECRET_" + value
	}
	return value
}

// SecurityCopyFrom copies credentials from the config file at path into c,
// without overwriting values c already has. Secret values are resolved through
// their $VAR references, so only references travel with the config.
func (c *Config) SecurityCopyFrom(path string) error {
	existing, err := LoadConfig(path)
	if err != nil {
		return err
	}
	if c.Dashboard.Token.IsZero() {
		c.Dashboard.Token = existing.Dashboard.Token
	}
	copySecureValues(reflect.ValueOf(&existing.Tools).Elem(), reflect.ValueOf(&c.Tools).Elem())
	copyModelSecrets(existing.ModelList, c.ModelList)
	for name, dst := range c.Channels {
		src := existing.Channels.Get(name)
		if src == nil || dst == nil {
			continue
		}
		srcDecoded, err := src.GetDecoded()
		if err != nil {
			continue
		}
		dstDecoded, err := dst.GetDecoded()
		if err != nil {
			continue
		}
		copySecureValues(reflect.ValueOf(srcDecoded).Elem(), reflect.ValueOf(dstDecoded).Elem())
	}
	return nil
}

// copyModelSecrets matches by model_name and duplicate index, not by slice
// position: a config edit may reorder models.
func copyModelSecrets(src, dst SecureModelList) {
	srcIndex := toNameIndex(src)
	dstByName := make(map[string][]*ModelConfig)
	for _, model := range dst {
		if model != nil {
			dstByName[model.ModelName] = append(dstByName[model.ModelName], model)
		}
	}
	for i, model := range src {
		if model == nil || len(model.APIKeys) == 0 {
			continue
		}
		idx := 0
		if _, rawIndex, ok := strings.Cut(srcIndex[i], ":"); ok {
			idx, _ = strconv.Atoi(rawIndex)
		}
		matches := dstByName[model.ModelName]
		if idx >= 0 && idx < len(matches) && len(matches[idx].APIKeys) == 0 {
			matches[idx].APIKeys = model.APIKeys
		}
	}
}

// toNameIndex returns one "model_name:duplicate_index" entry per model, in
// slice order.
func toNameIndex(list []*ModelConfig) []string {
	nameList := make([]string, 0, len(list))
	countMap := make(map[string]int)
	for _, model := range list {
		name := ""
		if model != nil {
			name = model.ModelName
		}
		index := countMap[name]
		nameList = append(nameList, fmt.Sprintf("%s:%d", name, index))
		countMap[name]++
	}
	return nameList
}

// copySecureValues copies SecureString/SecureStrings values from src into dst
// wherever dst is still zero. Channels are handled explicitly by
// SecurityCopyFrom because their settings are decoded at runtime.
func copySecureValues(src, dst reflect.Value) {
	if !src.IsValid() || !dst.IsValid() || src.Type() != dst.Type() {
		return
	}
	if src.Type() == secureStringType {
		if dst.CanSet() && dst.IsZero() {
			dst.Set(src)
		}
		return
	}
	if src.Type() == secureStringsType {
		if dst.CanSet() && dst.Len() == 0 {
			dst.Set(src)
		}
		return
	}
	if src.Type() == channelType {
		return
	}
	switch src.Kind() {
	case reflect.Pointer:
		if src.IsNil() || dst.IsNil() {
			return
		}
		copySecureValues(src.Elem(), dst.Elem())
	case reflect.Struct:
		for i := range src.NumField() {
			if src.Type().Field(i).PkgPath != "" {
				continue
			}
			copySecureValues(src.Field(i), dst.Field(i))
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < src.Len() && i < dst.Len(); i++ {
			copySecureValues(src.Index(i), dst.Index(i))
		}
	}
}
