package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// stripRemovedConfigFields drops keys that earlier builds wrote but the current
// schema no longer defines: the top-level heartbeat section, the cron tool
// section, the unattended approval switch, approval rules for the removed
// "automation" category, and remembered always-allow entries for the removed
// cron tool. loadConfig strict-decodes unknown fields, so an existing
// config.json would otherwise keep the gateway from starting; the next
// SaveConfig writes a clean file.
func stripRemovedConfigFields(data []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	// Reject trailing content so a malformed file is not silently accepted.
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected data after top-level JSON value")
		}
		return nil, err
	}

	delete(root, "heartbeat")

	tools, ok := root["tools"].(map[string]any)
	if !ok {
		return json.Marshal(root)
	}
	delete(tools, "cron")

	approval, ok := tools["approval"].(map[string]any)
	if ok {
		delete(approval, "unattended")
		if rules, ok := approval["rules"].([]any); ok {
			approval["rules"] = filterJSONObjects(rules, func(rule map[string]any) bool {
				// A rule with an explicit tool remains meaningful even if its
				// category is the removed automation one; only category-only
				// automation rules are retired.
				return rule["category"] != "automation" || rule["tool"] != nil
			})
		}
		if allow, ok := approval["always_allow"].([]any); ok {
			approval["always_allow"] = filterJSONObjects(allow, func(entry map[string]any) bool {
				return entry["tool"] != "cron"
			})
		}
	}
	return json.Marshal(root)
}

// decodeConfigJSON decodes migrated into target and reports failures against
// the original bytes so line/column diagnostics and source previews point at
// the file on disk. json.Unmarshal ignores the fields the migration removed, so
// a type error still surfaces at its real offset instead of the shifted one in
// the compacted migration output.
func decodeConfigJSON(original, migrated []byte, target any) error {
	err := decodeJSONWithDiagnostics(migrated, target, "config.json")
	if err == nil {
		return nil
	}
	if originalErr := json.Unmarshal(original, target); originalErr != nil {
		return wrapJSONError(original, originalErr, "config.json")
	}
	return err
}

// filterJSONObjects keeps entries accepted by keep. Entries that are not JSON
// objects are kept so the schema decoder can report them instead of the
// migration hiding the malformed value.
func filterJSONObjects(entries []any, keep func(map[string]any) bool) []any {
	filtered := entries[:0]
	for _, entry := range entries {
		obj, ok := entry.(map[string]any)
		if !ok || keep(obj) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
