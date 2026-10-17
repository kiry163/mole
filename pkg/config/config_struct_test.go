package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestSecureStringJSONKeepsReference(t *testing.T) {
	t.Setenv("TEST_SECRET", "s3cret")
	s := NewSecureString("$TEST_SECRET")
	data, err := json.Marshal(struct {
		Token SecureString `json:"token"`
	}{Token: *s})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"token":"$TEST_SECRET"}` {
		t.Fatalf("json = %s", data)
	}
}

func TestSecureStringJSONKeepsPlaintext(t *testing.T) {
	s := NewSecureString("plain")
	data, err := json.Marshal(struct {
		Token SecureString `json:"token"`
	}{Token: *s})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"token":"plain"}` {
		t.Fatalf("json = %s", data)
	}
}

func TestSecureStringsJSONKeepsReferences(t *testing.T) {
	t.Setenv("KEY_A", "a")
	t.Setenv("KEY_B", "b")
	values := SecureStrings{NewSecureString("$KEY_A"), NewSecureString("$KEY_B")}
	data, err := json.Marshal(struct {
		Keys SecureStrings `json:"keys"`
	}{Keys: values})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"keys":["$KEY_A","$KEY_B"]}` {
		t.Fatalf("json = %s", data)
	}
}

func TestSkillRegistryConfigDecodeParam(t *testing.T) {
	registry := SkillRegistryConfig{
		Name: "github",
		Param: map[string]any{
			"proxy": "http://127.0.0.1:7890",
		},
	}

	var private struct {
		Proxy string `json:"proxy"`
	}
	err := registry.DecodeParam(&private)
	assert.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:7890", private.Proxy)
}

func TestSkillRegistryConfigJSONFlattensParam(t *testing.T) {
	registry := SkillRegistryConfig{
		Name:    "github",
		Enabled: true,
		BaseURL: "https://github.com",
		Param: map[string]any{
			"proxy": "http://127.0.0.1:7890",
		},
	}

	data, err := json.Marshal(registry)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"proxy":"http://127.0.0.1:7890"`)
	assert.NotContains(t, string(data), `"param"`)

	var loaded SkillRegistryConfig
	err = json.Unmarshal(data, &loaded)
	assert.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:7890", loaded.Param["proxy"])
}

func TestSkillRegistryConfigJSONIgnoresShadowSecretFields(t *testing.T) {
	var registry SkillRegistryConfig
	err := json.Unmarshal([]byte(`{
		"enabled": true,
		"base_url": "https://github.com",
		"_auth_token": "shadow-secret",
		"proxy": "http://127.0.0.1:7890"
	}`), &registry)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com", registry.BaseURL)
	assert.Equal(t, "http://127.0.0.1:7890", registry.Param["proxy"])
	_, exists := registry.Param["_auth_token"]
	assert.False(t, exists)

	registry.Param["_auth_token"] = "should-not-round-trip"
	data, err := json.Marshal(registry)
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "_auth_token")
	assert.Contains(t, string(data), `"proxy":"http://127.0.0.1:7890"`)

	yamlData, err := yaml.Marshal(registry)
	assert.NoError(t, err)
	assert.NotContains(t, string(yamlData), "_auth_token")
	assert.Contains(t, string(yamlData), "proxy: http://127.0.0.1:7890")
}

func TestSkillRegistryConfigYAMLIgnoresShadowSecretFields(t *testing.T) {
	var registry SkillRegistryConfig
	err := yaml.Unmarshal([]byte(`
enabled: true
base_url: https://github.com
_auth_token: shadow-secret
proxy: http://127.0.0.1:7890
`), &registry)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com", registry.BaseURL)
	assert.Equal(t, "http://127.0.0.1:7890", registry.Param["proxy"])
	_, exists := registry.Param["_auth_token"]
	assert.False(t, exists)
}

func TestSkillsRegistriesConfigMarshalYAMLIncludesRegistryToken(t *testing.T) {
	registries := SkillsRegistriesConfig{
		&SkillRegistryConfig{
			Name:      "github",
			AuthToken: *NewSecureString("registry-auth-token"),
		},
	}

	data, err := yaml.Marshal(registries)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "github:")
	assert.Contains(t, string(data), "auth_token: registry-auth-token")

	loaded := SkillsRegistriesConfig{
		&SkillRegistryConfig{Name: "github"},
	}
	err = yaml.Unmarshal(data, &loaded)
	assert.NoError(t, err)
	github, ok := loaded.Get("github")
	assert.True(t, ok)
	assert.Equal(t, "registry-auth-token", github.AuthToken.String())
}

func TestSkillsRegistriesConfigUnmarshalYAMLBuildsEntriesFromEmptySlice(t *testing.T) {
	var registries SkillsRegistriesConfig
	err := yaml.Unmarshal([]byte(`github:
  enabled: true
  base_url: https://ghe.example.com/git
  proxy: http://127.0.0.1:7890
`), &registries)
	assert.NoError(t, err)

	github, ok := registries.Get("github")
	assert.True(t, ok)
	assert.True(t, github.Enabled)
	assert.Equal(t, "https://ghe.example.com/git", github.BaseURL)
	assert.Equal(t, "http://127.0.0.1:7890", github.Param["proxy"])
}

func TestSkillsRegistriesConfigMarshalJSONPreservesObjectShape(t *testing.T) {
	registries := SkillsRegistriesConfig{
		&SkillRegistryConfig{
			Name:    "github",
			Enabled: true,
			BaseURL: "https://ghe.example.com/git",
			Param: map[string]any{
				"proxy": "http://127.0.0.1:7890",
			},
		},
		&SkillRegistryConfig{
			Name:    "clawhub",
			Enabled: true,
			BaseURL: "https://clawhub.ai",
		},
	}

	data, err := json.Marshal(registries)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"github":{`)
	assert.Contains(t, string(data), `"clawhub":{`)
	assert.NotContains(t, string(data), `[{`)
	assert.NotContains(t, string(data), `"name":"github"`)
	assert.NotContains(t, string(data), `"name":"clawhub"`)

	var decoded map[string]json.RawMessage
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Contains(t, decoded, "github")
	assert.Contains(t, decoded, "clawhub")

	var roundTripped SkillsRegistriesConfig
	err = json.Unmarshal(data, &roundTripped)
	assert.NoError(t, err)

	github, ok := roundTripped.Get("github")
	assert.True(t, ok)
	assert.Equal(t, "https://ghe.example.com/git", github.BaseURL)
	assert.Equal(t, "http://127.0.0.1:7890", github.Param["proxy"])

	clawhub, ok := roundTripped.Get("clawhub")
	assert.True(t, ok)
	assert.Equal(t, "https://clawhub.ai", clawhub.BaseURL)
}

func TestSkillsRegistriesConfigUnmarshalJSONPreservesDefaultRegistries(t *testing.T) {
	registries := DefaultConfig().Tools.Skills.Registries

	err := json.Unmarshal([]byte(`{
		"clawhub": {
			"base_url": "https://clawhub.example.com"
		}
	}`), &registries)
	assert.NoError(t, err)

	clawhub, ok := registries.Get("clawhub")
	assert.True(t, ok)
	assert.True(t, clawhub.Enabled)
	assert.Equal(t, "https://clawhub.example.com", clawhub.BaseURL)

	github, ok := registries.Get("github")
	assert.True(t, ok)
	assert.True(t, github.Enabled)
	assert.Equal(t, "https://github.com", github.BaseURL)
	assert.Empty(t, github.Param)
}

func TestSkillsRegistriesConfigUnmarshalJSONListPreservesDefaultRegistries(t *testing.T) {
	registries := DefaultConfig().Tools.Skills.Registries

	err := json.Unmarshal([]byte(`[
		{
			"name": "clawhub",
			"base_url": "https://clawhub.example.com"
		}
	]`), &registries)
	assert.NoError(t, err)

	clawhub, ok := registries.Get("clawhub")
	assert.True(t, ok)
	assert.True(t, clawhub.Enabled)
	assert.Equal(t, "https://clawhub.example.com", clawhub.BaseURL)

	github, ok := registries.Get("github")
	assert.True(t, ok)
	assert.True(t, github.Enabled)
	assert.Equal(t, "https://github.com", github.BaseURL)
	assert.Empty(t, github.Param)
}

func TestSkillsRegistriesConfigUnmarshalYAMLAppendsNewRegistryToExistingSlice(t *testing.T) {
	registries := DefaultConfig().Tools.Skills.Registries

	err := yaml.Unmarshal([]byte(`custom:
  base_url: https://skills.example.com
  auth_token: custom-token
`), &registries)
	assert.NoError(t, err)

	custom, ok := registries.Get("custom")
	assert.True(t, ok)
	assert.Equal(t, "https://skills.example.com", custom.BaseURL)
	assert.Equal(t, "custom-token", custom.AuthToken.String())

	github, ok := registries.Get("github")
	assert.True(t, ok)
	assert.Equal(t, "https://github.com", github.BaseURL)
}

func TestSkillsRegistriesConfigUnmarshalYAMLOverridesDefaultRegistryFields(t *testing.T) {
	registries := DefaultConfig().Tools.Skills.Registries

	err := yaml.Unmarshal([]byte(`github:
  enabled: false
  base_url: https://ghe.example.com/git
  proxy: http://127.0.0.1:7890
`), &registries)
	assert.NoError(t, err)

	github, ok := registries.Get("github")
	assert.True(t, ok)
	assert.False(t, github.Enabled)
	assert.Equal(t, "https://ghe.example.com/git", github.BaseURL)
	assert.Equal(t, "http://127.0.0.1:7890", github.Param["proxy"])
}

func TestSkillsRegistriesConfigUnmarshalYAMLRetainsDefaultsForOmittedFields(t *testing.T) {
	registries := DefaultConfig().Tools.Skills.Registries

	err := yaml.Unmarshal([]byte(`github:
  auth_token: registry-token
`), &registries)
	assert.NoError(t, err)

	github, ok := registries.Get("github")
	assert.True(t, ok)
	assert.True(t, github.Enabled)
	assert.Equal(t, "https://github.com", github.BaseURL)
	assert.Equal(t, "registry-token", github.AuthToken.String())
	assert.Empty(t, github.Param)
}
