package agent

import (
	"context"

	"github.com/kiry163/mole/pkg/bus"
	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/providers"
	"github.com/kiry163/mole/pkg/tools"
)

func namedHook(name string, hook any) HookRegistration {
	return HookRegistration{Name: name, Source: HookSourceInProcess, Hook: hook}
}

func unregisterTestBuiltinHook(name string) {
	if name == "" {
		return
	}
	builtinHookRegistryMu.Lock()
	delete(builtinHookRegistry, name)
	builtinHookRegistryMu.Unlock()
}

func registerTool(al *AgentLoop, tool tools.Tool) {
	registry := al.GetRegistry()
	for _, agentID := range registry.ListAgentIDs() {
		if agent, ok := registry.GetAgent(agentID); ok {
			agent.Tools.Register(tool)
		}
	}
}

func processDirectForTest(
	al *AgentLoop,
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	if err := al.ensureHooksInitialized(ctx); err != nil {
		return "", err
	}
	if err := al.ensureMCPInitialized(ctx); err != nil {
		return "", err
	}

	return al.processMessage(ctx, bus.InboundMessage{
		Context: bus.InboundContext{
			Channel:  channel,
			ChatID:   chatID,
			ChatType: "direct",
			SenderID: "cli",
		},
		Content:    content,
		SessionKey: sessionKey,
	})
}

func buildMessagesForTest(
	cb *ContextBuilder,
	history []providers.Message,
	summary string,
	currentMessage string,
	media []string,
	channel, chatID, senderID, senderDisplayName string,
	activeSkills ...string,
) []providers.Message {
	return cb.BuildMessagesFromPrompt(PromptBuildRequest{
		History:           history,
		Summary:           summary,
		CurrentMessage:    currentMessage,
		Media:             media,
		Channel:           channel,
		ChatID:            chatID,
		SenderID:          senderID,
		SenderDisplayName: senderDisplayName,
		ActiveSkills:      append([]string(nil), activeSkills...),
	})
}

func populateCandidateProvidersFromNames(
	cfg *config.Config,
	names []string,
	out map[string]providers.LLMProvider,
) {
	if cfg == nil || len(names) == 0 {
		return
	}
	for _, name := range names {
		candidates := resolveModelCandidates(cfg, name, nil)
		populateCandidateProvidersFromCandidates(cfg, candidates, out)
	}
}

func newRuntimeEventLoggerForTest(cfg *config.Config) *runtimeEventLogger {
	logCfg := config.EffectiveEventLoggingConfig(cfg)
	if !logCfg.Enabled {
		return nil
	}
	return newRuntimeEventLoggerFromConfig(logCfg)
}
