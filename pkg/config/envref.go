package config

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var wholeEnvRefPattern = regexp.MustCompile(`^\$(?:\{([A-Za-z_][A-Za-z0-9_]*)\}|([A-Za-z_][A-Za-z0-9_]*))$`)

// envRef records a plain string that ExpandEnvRefs replaced, so a later
// SaveConfig can persist the reference instead of the resolved value.
type envRef struct {
	raw      string
	expanded string
}

// envRefPathKey joins a reflection path so it can be used as a map key
// without colliding with a field or map key containing the separator.
func envRefPathKey(path []string) string {
	return strings.Join(path, "\x00")
}

// ExpandEnvRefs expands whole-string $VAR / ${VAR} references in the config.
// Embedded references, recursion, and non-string fields are not expanded.
// Every expanded plain string is remembered so RestoreEnvRefs can put the
// reference back before the config is serialized.
func ExpandEnvRefs(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	cfg.envRefs = make(map[string]envRef)
	return expandEnvReflect(reflect.ValueOf(cfg).Elem(), nil, cfg.envRefs)
}

func expandEnvReflect(v reflect.Value, path []string, refs map[string]envRef) error {
	if !v.IsValid() {
		return nil
	}
	if v.Type() == reflect.TypeOf(SecureString{}) ||
		v.Type() == reflect.TypeOf(SecureStrings{}) ||
		v.Type() == reflect.TypeOf(Channel{}) {
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return expandEnvReflect(v.Elem(), path, refs)
	case reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return expandEnvReflect(v.Elem(), path, refs)
	case reflect.Struct:
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			if err := expandEnvReflect(v.Field(i), append(path, field.Name), refs); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if err := expandEnvReflect(v.Index(i), append(path, strconv.Itoa(i)), refs); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			elem := v.MapIndex(key)
			if elem.Kind() != reflect.String {
				// map values other than string are not mutated in place
				continue
			}
			keyPath := append(path, fmt.Sprint(key.Interface()))
			raw := elem.String()
			expanded, err := expandEnvString(raw)
			if err != nil {
				return err
			}
			if expanded != raw {
				refs[envRefPathKey(keyPath)] = envRef{raw: raw, expanded: expanded}
			}
			v.SetMapIndex(key, reflect.ValueOf(expanded).Convert(elem.Type()))
		}
	case reflect.String:
		raw := v.String()
		expanded, err := expandEnvString(raw)
		if err != nil {
			return err
		}
		if expanded != raw {
			v.SetString(expanded)
			refs[envRefPathKey(path)] = envRef{raw: raw, expanded: expanded}
		}
	}
	return nil
}

// RestoreEnvRefs puts recorded $VAR references back where the current value is
// still the expanded one. Values changed since LoadConfig are left alone: the
// user meant them.
func (c *Config) RestoreEnvRefs() {
	if c == nil || len(c.envRefs) == 0 {
		return
	}
	_ = restoreEnvReflect(reflect.ValueOf(c).Elem(), nil, c.envRefs)
}

func restoreEnvReflect(v reflect.Value, path []string, refs map[string]envRef) error {
	if !v.IsValid() {
		return nil
	}
	if v.Type() == reflect.TypeOf(SecureString{}) ||
		v.Type() == reflect.TypeOf(SecureStrings{}) ||
		v.Type() == reflect.TypeOf(Channel{}) {
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return restoreEnvReflect(v.Elem(), path, refs)
	case reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return restoreEnvReflect(v.Elem(), path, refs)
	case reflect.Struct:
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			if err := restoreEnvReflect(v.Field(i), append(path, field.Name), refs); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if err := restoreEnvReflect(v.Index(i), append(path, strconv.Itoa(i)), refs); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			elem := v.MapIndex(key)
			if elem.Kind() != reflect.String {
				continue
			}
			keyPath := append(path, fmt.Sprint(key.Interface()))
			ref, ok := refs[envRefPathKey(keyPath)]
			if !ok || elem.String() != ref.expanded {
				continue
			}
			v.SetMapIndex(key, reflect.ValueOf(ref.raw).Convert(elem.Type()))
		}
	case reflect.String:
		ref, ok := refs[envRefPathKey(path)]
		if !ok || v.String() != ref.expanded {
			return nil
		}
		v.SetString(ref.raw)
	}
	return nil
}

// ValidateEnvRefs rejects whole-string $VAR references to variables that are
// not defined. It runs on save so a submitted reference cannot be persisted
// only to break the next load. SecureString and Channel values resolve
// themselves, so they are skipped.
func ValidateEnvRefs(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	return validateEnvReflect(reflect.ValueOf(cfg).Elem())
}

func validateEnvReflect(v reflect.Value) error {
	if !v.IsValid() {
		return nil
	}
	if v.Type() == reflect.TypeOf(SecureString{}) ||
		v.Type() == reflect.TypeOf(SecureStrings{}) ||
		v.Type() == reflect.TypeOf(Channel{}) {
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return validateEnvReflect(v.Elem())
	case reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return validateEnvReflect(v.Elem())
	case reflect.Struct:
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			if err := validateEnvReflect(v.Field(i)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if err := validateEnvReflect(v.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			elem := v.MapIndex(key)
			if elem.Kind() == reflect.String {
				if err := validateEnvRefString(elem.String()); err != nil {
					return err
				}
				continue
			}
			elem = reflect.ValueOf(elem.Interface())
			if err := validateEnvReflect(elem); err != nil {
				return err
			}
		}
	case reflect.String:
		return validateEnvRefString(v.String())
	}
	return nil
}

func validateEnvRefString(value string) error {
	if strings.HasPrefix(value, "$$") {
		return nil
	}
	match := wholeEnvRefPattern.FindStringSubmatch(value)
	if match == nil {
		return nil
	}
	name := match[1]
	if name == "" {
		name = match[2]
	}
	if _, ok := os.LookupEnv(name); !ok {
		return fmt.Errorf("config: undefined environment variable %s", name)
	}
	return nil
}

func expandEnvString(value string) (string, error) {
	if strings.HasPrefix(value, "$$") {
		return value[1:], nil
	}
	resolved, ok, err := lookupWholeEnvRef(value)
	if err != nil {
		return "", err
	}
	if !ok {
		return value, nil
	}
	return resolved, nil
}

func lookupWholeEnvRef(value string) (string, bool, error) {
	match := wholeEnvRefPattern.FindStringSubmatch(value)
	if match == nil {
		return "", false, nil
	}
	name := match[1]
	if name == "" {
		name = match[2]
	}
	got, ok := os.LookupEnv(name)
	if !ok {
		return "", true, fmt.Errorf("config: undefined environment variable %s", name)
	}
	return got, true, nil
}
