package agent

import (
	"strings"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/providers"
)

func promptBuildRequestForTurn(
	ts *turnState,
	history []providers.Message,
	summary string,
	currentMessage string,
	media []string,
	cfg *config.Config,
) PromptBuildRequest {
	req := PromptBuildRequest{
		History:           history,
		Summary:           summary,
		CurrentMessage:    currentMessage,
		Media:             append([]string(nil), media...),
		Channel:           ts.channel,
		ChatID:            ts.chatID,
		SenderID:          ts.opts.Dispatch.SenderID(),
		SenderDisplayName: ts.opts.SenderDisplayName,
		ActiveSkills:      activeSkillNames(ts.agent, ts.opts),
	}
	return req
}

func promptBuildRequestForProcessOptions(
	agent *AgentInstance,
	opts processOptions,
	history []providers.Message,
	summary string,
	currentMessage string,
	media []string,
) PromptBuildRequest {
	req := PromptBuildRequest{
		History:           history,
		Summary:           summary,
		CurrentMessage:    currentMessage,
		Media:             append([]string(nil), media...),
		Channel:           opts.Channel,
		ChatID:            opts.ChatID,
		SenderID:          opts.SenderID,
		SenderDisplayName: opts.SenderDisplayName,
		ActiveSkills:      activeSkillNames(agent, opts),
	}
	return req
}

func promptContentBlock(part PromptPart) providers.ContentBlock {
	return providers.ContentBlock{
		Type:         "text",
		Text:         part.Content,
		PromptLayer:  string(part.Layer),
		PromptSlot:   string(part.Slot),
		PromptSource: string(part.Source.ID),
	}
}

func promptMessageWithMetadata(
	msg providers.Message,
	layer PromptLayer,
	slot PromptSlot,
	source PromptSourceID,
) providers.Message {
	msg.PromptLayer = string(layer)
	msg.PromptSlot = string(slot)
	msg.PromptSource = string(source)
	return msg
}

func promptMessageWithDefaultMetadata(
	msg providers.Message,
	layer PromptLayer,
	slot PromptSlot,
	source PromptSourceID,
) providers.Message {
	if strings.TrimSpace(msg.PromptSource) != "" {
		return msg
	}
	return promptMessageWithMetadata(msg, layer, slot, source)
}

func userPromptMessage(content string, media []string) providers.Message {
	msg := providers.Message{
		Role:    "user",
		Content: content,
	}
	if len(media) > 0 {
		msg.Media = append([]string(nil), media...)
	}
	return promptMessageWithMetadata(msg, PromptLayerTurn, PromptSlotMessage, PromptSourceUserMessage)
}

func toolResultPromptMessage(content, toolCallID string, media []string) providers.Message {
	msg := providers.Message{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolCallID,
	}
	if len(media) > 0 {
		msg.Media = append([]string(nil), media...)
	}
	return promptMessageWithMetadata(msg, PromptLayerTurn, PromptSlotToolResult, PromptSourceToolResult)
}

func toolImageFollowUpPromptMessage(media []string) providers.Message {
	msg := providers.Message{
		Role:    "user",
		Content: "[Loaded image from tool result above]",
	}
	if len(media) > 0 {
		msg.Media = append([]string(nil), media...)
	}
	return promptMessageWithMetadata(msg, PromptLayerTurn, PromptSlotToolResult, PromptSourceToolResult)
}

func steeringPromptMessage(msg providers.Message) providers.Message {
	return promptMessageWithDefaultMetadata(msg, PromptLayerTurn, PromptSlotSteering, PromptSourceSteering)
}

func interruptPromptMessage(content string) providers.Message {
	return promptMessageWithMetadata(
		providers.Message{Role: "user", Content: content},
		PromptLayerTurn,
		PromptSlotInterrupt,
		PromptSourceInterrupt,
	)
}
