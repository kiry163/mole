package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ─── Test extend structs (simplified, settings + secure in one struct) ───

type testTelegramConfig struct {
	BaseURL       string          `json:"base_url"            yaml:"-"`
	Proxy         string          `json:"proxy"               yaml:"-"`
	UseMarkdownV2 bool            `json:"use_markdown_v2"     yaml:"-"`
	Streaming     StreamingConfig `json:"streaming,omitempty" yaml:"-"`
	Token         SecureString    `json:"token,omitzero"      yaml:"token,omitempty"`
}

type testAuxChannelConfig struct {
	MentionOnly bool          `json:"mention_only"      yaml:"-"`
	Token       SecureString  `json:"token,omitzero"    yaml:"token,omitempty"`
	ApiKeys     SecureStrings `json:"api_keys,omitzero" yaml:"api_keys,omitempty"`
}

// ═══════════════════════════════════════════════════
//  RawNode JSON/YAML round-trip
// ═══════════════════════════════════════════════════

func TestRawNode_JSON_RoundTrip(t *testing.T) {
	t.Run("unmarshal and decode", func(t *testing.T) {
		var r RawNode
		require.NoError(t, json.Unmarshal([]byte(`{"key":"value","num":42}`), &r))
		assert.False(t, r.IsEmpty())

		var m map[string]any
		require.NoError(t, r.Decode(&m))
		assert.Equal(t, "value", m["key"])
		assert.Equal(t, float64(42), m["num"])
	})

	t.Run("marshal round-trip", func(t *testing.T) {
		r := RawNode(`{"a":1}`)
		data, err := json.Marshal(r)
		require.NoError(t, err)
		assert.JSONEq(t, `{"a":1}`, string(data))
	})

	t.Run("null input", func(t *testing.T) {
		var r RawNode
		require.NoError(t, json.Unmarshal([]byte("null"), &r))
		assert.True(t, r.IsEmpty())

		data, err := json.Marshal(r)
		require.NoError(t, err)
		assert.Equal(t, "null", string(data))
	})

	t.Run("empty node decode", func(t *testing.T) {
		var r RawNode
		var m map[string]any
		require.NoError(t, r.Decode(&m))
		assert.Nil(t, m)
	})
}

func TestRawNode_YAML_RoundTrip(t *testing.T) {
	t.Run("unmarshal and decode", func(t *testing.T) {
		var r RawNode
		require.NoError(t, yaml.Unmarshal([]byte("key: value\nnum: 42"), &r))
		assert.False(t, r.IsEmpty())

		var m map[string]any
		require.NoError(t, r.Decode(&m))
		assert.Equal(t, "value", m["key"])
	})

	t.Run("marshal round-trip", func(t *testing.T) {
		r := RawNode(`{"name":"test"}`)
		data, err := yaml.Marshal(r)
		require.NoError(t, err)
		assert.Contains(t, string(data), "name: test")
	})

	t.Run("empty node marshal", func(t *testing.T) {
		var r RawNode
		v, err := yaml.Marshal(r)
		require.NoError(t, err)
		assert.Equal(t, "null\n", string(v))
	})
}

// ═══════════════════════════════════════════════════
//  JSON unmarshal: extend.json
// ═══════════════════════════════════════════════════

func TestChannel_JSON_Unmarshal(t *testing.T) {
	jsonData := `{
		"enabled": true,
		"type": "telegram",
		"allow_from": ["user1", "user2"],
		"reasoning_channel_id": "-100xxx",
		"settings": {
			"base_url": "https://custom-api.example.com",
			"use_markdown_v2": true,
			"streaming": {"enabled": true, "throttle_seconds": 2},
			"token": "[NOT_HERE]"
		}
	}`

	var ch Channel
	require.NoError(t, json.Unmarshal([]byte(jsonData), &ch))

	assert.True(t, ch.Enabled)
	assert.Equal(t, "telegram", ch.Type)
	assert.Equal(t, FlexibleStringSlice{"user1", "user2"}, ch.AllowFrom)
	assert.Equal(t, "-100xxx", ch.ReasoningChannelID)
	assert.False(t, ch.SettingsIsEmpty())

	// Decode into combined struct
	var cfg testTelegramConfig
	require.NoError(t, ch.Decode(&cfg))
	assert.Equal(t, "https://custom-api.example.com", cfg.BaseURL)
	assert.True(t, cfg.UseMarkdownV2)
	assert.True(t, cfg.Streaming.Enabled)
	assert.Equal(t, 2, cfg.Streaming.ThrottleSeconds)
	// SecureString.UnmarshalJSON("[NOT_HERE]") → no-op → empty
	assert.Equal(t, "", cfg.Token.String())
}

func TestStreamingConfig_IsChannelGeneric(t *testing.T) {
	typ := reflect.TypeOf(StreamingConfig{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if got := field.Tag.Get("env"); got != "" {
			t.Fatalf("StreamingConfig.%s env tag = %q, want no channel-specific env tag", field.Name, got)
		}
	}
}

func TestWebChatSettings_StreamingConfig(t *testing.T) {
	raw := RawNode(`{
		"token": "test-token",
		"streaming": {
			"enabled": true,
			"throttle_seconds": 2,
			"min_growth_chars": 80
		}
	}`)
	ch := &Channel{
		Type:     ChannelWebChat,
		Enabled:  true,
		Settings: raw,
	}
	ch.SetName("webchat")
	var webchatCfg WebChatSettings
	if err := ch.Decode(&webchatCfg); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	assert.True(t, webchatCfg.Streaming.Enabled)
	assert.Equal(t, 2, webchatCfg.Streaming.ThrottleSeconds)
	assert.Equal(t, 80, webchatCfg.Streaming.MinGrowthChars)
}

func TestWebChatStreamingConfig_Defaults(t *testing.T) {
	cfg := StreamingConfig{Enabled: true}
	got := cfg.WithDefaults(1, 40)
	assert.Equal(t, 1, got.ThrottleSeconds)
	assert.Equal(t, 40, got.MinGrowthChars)

	cfg = StreamingConfig{Enabled: true, ThrottleSeconds: 5, MinGrowthChars: 200}
	got = cfg.WithDefaults(1, 40)
	assert.Equal(t, 5, got.ThrottleSeconds)
	assert.Equal(t, 200, got.MinGrowthChars)

	cfg = StreamingConfig{}
	got = cfg.WithDefaults(1, 40)
	assert.Equal(t, 0, got.ThrottleSeconds)
	assert.Equal(t, 0, got.MinGrowthChars)
}

func TestInitChannelList_RejectsNegativeStreamingDeliveryValues(t *testing.T) {
	tests := []struct {
		name        string
		channelType string
		settings    string
	}{
		{
			name:        "webchat throttle",
			channelType: ChannelWebChat,
			settings:    `{"token":"webchat-token","streaming":{"enabled":true,"throttle_seconds":-1}}`,
		},
		{
			name:        "webchat growth",
			channelType: ChannelWebChat,
			settings:    `{"token":"webchat-token","streaming":{"enabled":true,"min_growth_chars":-1}}`,
		},
		{
			name:        "telegram throttle",
			channelType: ChannelTelegram,
			settings:    `{"token":"telegram-token","streaming":{"enabled":true,"throttle_seconds":-1}}`,
		},
		{
			name:        "telegram growth",
			channelType: ChannelTelegram,
			settings:    `{"token":"telegram-token","streaming":{"enabled":true,"min_growth_chars":-1}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channels := ChannelsConfig{
				tt.channelType: {
					Type:     tt.channelType,
					Enabled:  true,
					Settings: RawNode(tt.settings),
				},
			}
			err := InitChannelList(channels)
			if err == nil {
				t.Fatal("InitChannelList() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), "streaming.") {
				t.Fatalf("InitChannelList() error = %v, want streaming field error", err)
			}
		})
	}
}

// ═══════════════════════════════════════════════════
//  JSON marshal: secure fields keep their raw values
// ═══════════════════════════════════════════════════

func TestChannel_JSON_Marshal_KeepsSecureValues(t *testing.T) {
	ch := Channel{
		Enabled: true,
		Type:    ChannelTelegram,
		name:    "my_telegram",
		Settings: mustParseRawNode(
			`{"base_url": "https://api.telegram.org", "proxy": "socks5://127.0.0.1:1080", "token": "123456:SECRET"}`,
		),
	}
	// Decode to register secure field names
	var cfg testTelegramConfig
	require.NoError(t, ch.Decode(&cfg))

	data, err := json.MarshalIndent(ch, "", "  ")
	require.NoError(t, err)
	t.Logf("JSON output:\n%s", string(data))

	assert.Contains(t, string(data), `"token": "123456:SECRET"`)
	assert.Contains(t, string(data), "base_url")
	assert.Contains(t, string(data), "proxy")
}

func TestChannel_JSON_Marshal_OmitsUnconfiguredStreaming(t *testing.T) {
	ch := Channel{
		Enabled:  true,
		Type:     ChannelWebChat,
		name:     "webchat",
		Settings: mustParseRawNode(`{"ping_interval":30}`),
	}
	var cfg WebChatSettings
	require.NoError(t, ch.Decode(&cfg))

	data, err := json.MarshalIndent(ch, "", "  ")
	require.NoError(t, err)

	assert.NotContains(t, string(data), `"streaming"`)
}

// ═══════════════════════════════════════════════════
//  extractSecureFieldNames
// ═══════════════════════════════════════════════════

func TestExtractSecureFieldNames(t *testing.T) {
	t.Run("telegram extend", func(t *testing.T) {
		names := extractSecureFieldNames(&testTelegramConfig{})
		assert.Equal(t, map[string]struct{}{"token": {}}, names)
	})

	t.Run("aux extend", func(t *testing.T) {
		names := extractSecureFieldNames(&testAuxChannelConfig{})
		assert.Equal(t, map[string]struct{}{"token": {}, "api_keys": {}}, names)
	})

	t.Run("non-struct target", func(t *testing.T) {
		names := extractSecureFieldNames("not a struct")
		assert.Nil(t, names)
	})

	t.Run("struct without secure fields", func(t *testing.T) {
		type NoSecure struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		names := extractSecureFieldNames(&NoSecure{})
		assert.Empty(t, names)
	})
}

// ═══════════════════════════════════════════════════
//  mergeRawJSON
// ═══════════════════════════════════════════════════

func TestMergeRawJSON(t *testing.T) {
	t.Run("overlay overrides base", func(t *testing.T) {
		base := RawNode(`{"base_url": "old", "token": "[NOT_HERE]"}`)
		overlay := RawNode(`{"token": "REAL_TOKEN"}`)
		merged, err := mergeRawJSON(base, overlay)
		require.NoError(t, err)

		var m map[string]any
		json.Unmarshal(merged, &m)
		assert.Equal(t, "old", m["base_url"])
		assert.Equal(t, "REAL_TOKEN", m["token"])
	})

	t.Run("empty overlay", func(t *testing.T) {
		base := RawNode(`{"base_url": "https://api.telegram.org"}`)
		merged, err := mergeRawJSON(base, nil)
		require.NoError(t, err)
		// mergeRawJSON normalizes JSON through unmarshal→marshal, so compare parsed values
		var orig, result map[string]any
		json.Unmarshal(base, &orig)
		json.Unmarshal(merged, &result)
		assert.Equal(t, orig, result)
	})

	t.Run("empty base", func(t *testing.T) {
		overlay := RawNode(`{"token": "NEW"}`)
		merged, err := mergeRawJSON(nil, overlay)
		require.NoError(t, err)
		assert.Contains(t, string(merged), `"token":"NEW"`)
	})
}

// ═══════════════════════════════════════════════════
//  Empty/missing settings
// ═══════════════════════════════════════════════════

func TestChannel_EmptySettings(t *testing.T) {
	// Flat format with only common fields: enabled and type are extracted to Channel,
	// Settings should be empty (no channel-specific fields)
	jsonData := `{
		"enabled": true,
		"type": "telegram"
	}`

	var ch Channel
	require.NoError(t, json.Unmarshal([]byte(jsonData), &ch))
	// All fields are common fields — Settings should be empty
	assert.True(t, ch.SettingsIsEmpty())

	// Decode into typed config — common fields like enabled/type are extracted,
	// channel-specific fields should be empty
	var cfg testTelegramConfig
	require.NoError(t, ch.Decode(&cfg))
	assert.Equal(t, "", cfg.BaseURL)
	assert.Equal(t, "", cfg.Token.String())
}

func TestChannel_NestedEmptySettings(t *testing.T) {
	// Nested format with empty settings
	jsonData := `{
		"enabled": true,
		"type": "telegram",
		"settings": {}
	}`

	var ch Channel
	require.NoError(t, json.Unmarshal([]byte(jsonData), &ch))
	assert.True(t, ch.SettingsIsEmpty())

	var cfg testTelegramConfig
	require.NoError(t, ch.Decode(&cfg))
	assert.Equal(t, "", cfg.BaseURL)
	assert.Equal(t, "", cfg.Token.String())
}

// ═══════════════════════════════════════════════════
//  removeSecureFields / filterSecureFields unit tests
// ═══════════════════════════════════════════════════

func TestRemoveSecureFields(t *testing.T) {
	t.Run("removes known secure fields", func(t *testing.T) {
		r := RawNode(`{"base_url": "https://api.telegram.org", "token": "SECRET"}`)
		names := map[string]struct{}{"token": {}}
		cleaned := removeSecureFields(r, names)

		var m map[string]any
		json.Unmarshal(cleaned, &m)
		assert.Equal(t, "https://api.telegram.org", m["base_url"])
		assert.NotContains(t, m, "token")
	})

	t.Run("nil secureFields returns as-is", func(t *testing.T) {
		r := RawNode(`{"token": "SECRET"}`)
		cleaned := removeSecureFields(r, nil)
		assert.Equal(t, string(r), string(cleaned))
	})

	t.Run("empty raw returns as-is", func(t *testing.T) {
		cleaned := removeSecureFields(nil, map[string]struct{}{"token": {}})
		assert.Nil(t, cleaned)
	})
}

func TestFilterSecureFields(t *testing.T) {
	t.Run("keeps only secure fields", func(t *testing.T) {
		r := RawNode(`{"base_url": "https://api.telegram.org", "token": "SECRET"}`)
		names := map[string]struct{}{"token": {}}
		filtered := filterSecureFields(r, names)

		var m map[string]any
		json.Unmarshal(filtered, &m)
		assert.NotContains(t, m, "base_url")
		assert.Equal(t, "SECRET", m["token"])
	})

	t.Run("nil secureFields returns nil", func(t *testing.T) {
		r := RawNode(`{"token": "SECRET"}`)
		filtered := filterSecureFields(r, nil)
		assert.Nil(t, filtered)
	})

	t.Run("empty raw returns nil", func(t *testing.T) {
		filtered := filterSecureFields(nil, map[string]struct{}{"token": {}})
		assert.Nil(t, filtered)
	})
}

func TestChannel_SecureStrings_ApiKeys_NoMerge(t *testing.T) {
	// JSON only, no merge — SecureStrings should be empty
	jsonData := `{
		"enabled": true,
		"type": "aux",
		"settings": {
			"mention_only": true,
			"token": "[NOT_HERE]",
			"api_keys": ["[NOT_HERE]"]
		}
	}`
	var ch Channel
	require.NoError(t, json.Unmarshal([]byte(jsonData), &ch))

	var cfg testAuxChannelConfig
	require.NoError(t, ch.Decode(&cfg))
	assert.True(t, cfg.MentionOnly)
	assert.Equal(t, "", cfg.Token.String())
	// ["[NOT_HERE]"] entries are filtered out → nil
	assert.Nil(t, cfg.ApiKeys)
}

// ─── helper ───

func mustParseRawNode(s string) RawNode {
	return RawNode(s)
}
