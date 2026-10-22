package channels

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/logger"
)

func TestToChannelHashes(t *testing.T) {
	logger.SetLevel(logger.DEBUG)
	cfg := config.DefaultConfig()
	results := toChannelHashes(cfg)
	assert.Equal(t, 0, len(results))
	logger.Debugf("results: %v", results)

	cfg2 := config.DefaultConfig()
	cfg2.Channels["channel_a"] = &config.Channel{
		Enabled:  true,
		Type:     config.ChannelQQ,
		Settings: config.RawNode(`{"enabled":true}`),
	}
	results2 := toChannelHashes(cfg2)
	assert.Equal(t, 1, len(results2))
	logger.Debugf("results2: %v", results2)
	added, removed := compareChannels(results, results2)
	assert.EqualValues(t, []string{"channel_a"}, added)
	assert.EqualValues(t, []string(nil), removed)

	cfg3 := config.DefaultConfig()
	cfg3.Channels["channel_b"] = &config.Channel{
		Enabled:  true,
		Type:     config.ChannelWeixin,
		Settings: config.RawNode(`{"enabled":true,"token":"test-token"}`),
	}
	results3 := toChannelHashes(cfg3)
	assert.Equal(t, 1, len(results3))
	logger.Debugf("results3: %v", results3)
	added, removed = compareChannels(results2, results3)
	assert.EqualValues(t, []string{"channel_a"}, removed)
	assert.EqualValues(t, []string{"channel_b"}, added)

	cfg3.Channels["channel_b"] = &config.Channel{
		Enabled:  true,
		Type:     config.ChannelWeixin,
		Settings: config.RawNode(`{"enabled":true,"token":"114314"}`),
	}
	results4 := toChannelHashes(cfg3)
	assert.Equal(t, 1, len(results4))
	logger.Debugf("results4: %v", results4)
	added, removed = compareChannels(results3, results4)
	assert.EqualValues(t, []string{"channel_b"}, removed)
	assert.EqualValues(t, []string{"channel_b"}, added)

	cc, err := toChannelConfig(cfg3, added)
	assert.NoError(t, err)
	bc := cc.Get("channel_b")
	assert.NotNil(t, bc)
	var wc config.WeixinSettings
	bc.Decode(&wc)
	assert.Equal(t, "114314", wc.Token.String())
	assert.Equal(t, true, bc.Enabled)
}

func TestToChannelHashes_RealWorldChannel(t *testing.T) {
	cfg := config.DefaultConfig()

	settings, _ := json.Marshal(map[string]any{
		"enabled": true,
		"token":   "weixin-token",
	})
	cfg.Channels["weixin"] = &config.Channel{
		Enabled:  true,
		Type:     config.ChannelWeixin,
		Settings: config.RawNode(settings),
	}

	h := toChannelHashes(cfg)
	assert.Equal(t, 1, len(h))
	assert.Contains(t, h, "weixin")
}

func TestToChannelHashes_MissingEnabledKey(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels["test"] = &config.Channel{
		Settings: config.RawNode(`{"key":"value"}`),
	}

	// Should not panic — the ok check safely handles the missing/false case
	assert.NotPanics(t, func() {
		_ = toChannelHashes(cfg)
	})
	h := toChannelHashes(cfg)
	assert.Equal(t, 0, len(h), "channel with Enabled=false (default) skipped")
}

func TestToChannelHashes_EnabledNotBool(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels["test"] = &config.Channel{
		Enabled:  false,
		Settings: config.RawNode(`{"enabled":"yes","boolField":true}`),
	}

	// Should not panic — string "enabled" won't match bool assertion, ok=false
	assert.NotPanics(t, func() {
		_ = toChannelHashes(cfg)
	})
	h := toChannelHashes(cfg)
	assert.Equal(t, 0, len(h), "string enabled not treated as true")
}
