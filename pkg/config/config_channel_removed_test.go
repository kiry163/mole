package config

import "testing"

// 既有用户的 config.json 里可能仍写着已删除的渠道类型。
// 契约：必须返回明确错误，且错误信息同时点出渠道名与该类型，便于人工修正配置。
func TestInitChannelList_RejectsRemovedChannelType(t *testing.T) {
	removed := []string{
		"discord", "slack", "slack_webhook", "teams_webhook", "matrix",
		"deltachat", "dingtalk", "line", "onebot", "irc", "vk", "maixcam",
		"whatsapp", "whatsapp_native", "mqtt", "wecom", "webchat_client",
	}
	for _, name := range removed {
		t.Run(name, func(t *testing.T) {
			channels := ChannelsConfig{name: {Enabled: true}}
			err := InitChannelList(channels)
			if err == nil {
				t.Fatalf("removed channel type %q must be rejected, got nil error", name)
			}
			want := `channel "` + name + `" has unknown type "` + name + `"`
			if err.Error() != want {
				t.Fatalf("error mismatch\n got: %s\nwant: %s", err.Error(), want)
			}
		})
	}
}

// 契约：webchat 仍是唯一的 UI 通道且仍是单例；webchat_client 不再是已知类型。
func TestChannelSingletons_AfterReduction(t *testing.T) {
	if !IsSingletonChannel(ChannelWebChat) {
		t.Fatal("webchat must stay a singleton channel")
	}
	for _, gone := range []string{"webchat_client", "discord", "matrix", "wecom"} {
		if IsSingletonChannel(gone) {
			t.Fatalf("%q must no longer be a singleton channel", gone)
		}
		if isValidChannelType(gone) {
			t.Fatalf("%q must no longer be a known channel type", gone)
		}
	}
}
