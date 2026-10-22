// Mole - Ultra-lightweight personal AI agent

package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kiry163/mole/pkg/agent/interfaces"
	"github.com/kiry163/mole/pkg/bus"
	"github.com/kiry163/mole/pkg/channels"
	"github.com/kiry163/mole/pkg/commands"
	"github.com/kiry163/mole/pkg/config"
	runtimeevents "github.com/kiry163/mole/pkg/events"
	"github.com/kiry163/mole/pkg/logger"
	"github.com/kiry163/mole/pkg/providers"
	"github.com/kiry163/mole/pkg/tools"
)

func NewAgentLoop(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	provider providers.LLMProvider,
) *AgentLoop {
	registry := NewAgentRegistry(cfg, provider)

	// Set up shared fallback chain with rate limiting.
	cooldown := providers.NewCooldownTracker()
	rl := providers.NewRateLimiterRegistry()
	// Register rate limiters for all agents' candidates so that RPM limits
	// configured in ModelConfig are enforced before each LLM call.
	if agent := registry.GetDefaultAgent(); agent != nil {
		rl.RegisterCandidates(agent.Candidates)
	}
	fallbackChain := providers.NewFallbackChain(cooldown, rl)

	// Determine worker pool size from config (default: 1 = sequential)
	workerPoolSize := cfg.Agents.Defaults.MaxParallelTurns
	if workerPoolSize <= 0 {
		workerPoolSize = 1
	}

	al := &AgentLoop{
		bus:           msgBus,
		cfg:           cfg,
		registry:      registry,
		fallback:      fallbackChain,
		cmdRegistry:   commands.NewRegistry(commands.BuiltinDefinitions()),
		steering:      newSteeringQueue(parseSteeringMode(cfg.Agents.Defaults.SteeringMode)),
		workerSem:     make(chan struct{}, workerPoolSize),
		runtimeEvents: runtimeevents.NewBus(),
	}
	al.activeReqCond = sync.NewCond(&al.activeReqMu)
	al.refreshRuntimeEventLogger(cfg)
	al.providerFactory = providers.CreateProviderFromConfig
	al.contextManager = al.resolveContextManager()

	// Register shared tools to all agents (now that al is created)
	registerSharedTools(al, cfg, msgBus, registry)

	return al
}

func registerSharedTools(
	al *AgentLoop,
	cfg *config.Config,
	msgBus interfaces.MessageBus,
	registry *AgentRegistry,
) {
	allowReadPaths := buildAllowReadPatterns(cfg)

	for _, agentID := range registry.ListAgentIDs() {
		agent, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}

		if cfg.Tools.IsToolEnabled("web") {
			searchTool, err := tools.NewWebSearchTool(tools.WebSearchToolOptionsFromConfig(cfg))
			if err != nil {
				logger.ErrorCF("agent", "Failed to create web search tool", map[string]any{"error": err.Error()})
			} else if searchTool != nil {
				agent.Tools.Register(searchTool)
			}
		}
		if cfg.Tools.IsToolEnabled("web_fetch") {
			fetchTool, err := tools.NewWebFetchToolWithProxy(
				50000,
				cfg.Tools.Web.Proxy,
				cfg.Tools.Web.Format,
				cfg.Tools.Web.FetchLimitBytes,
				cfg.Tools.Web.PrivateHostWhitelist)
			if err != nil {
				logger.ErrorCF("agent", "Failed to create web fetch tool", map[string]any{"error": err.Error()})
			} else {
				agent.Tools.Register(fetchTool)
			}
		}

		// Message tool
		if cfg.Tools.IsToolEnabled("message") {
			messageTool := tools.NewMessageTool()
			if cfg.Tools.Message.MediaEnabled {
				messageTool.ConfigureLocalMedia(
					agent.Workspace,
					cfg.Agents.Defaults.RestrictToWorkspace,
					cfg.Agents.Defaults.GetMaxMediaSize(),
					allowReadPaths,
				)
			}
			messageTool.SetSendCallback(func(
				ctx context.Context,
				channel, chatID, content, replyToMessageID string,
				mediaParts []bus.MediaPart,
			) error {
				outboundCtx := bus.NewOutboundContext(channel, chatID, replyToMessageID)
				outboundAgentID, outboundSessionKey, outboundScope := outboundTurnMetadata(
					tools.ToolAgentID(ctx),
					tools.ToolSessionKey(ctx),
					tools.ToolSessionScope(ctx),
				)
				if len(mediaParts) > 0 {
					outboundMedia := bus.OutboundMediaMessage{
						Channel:    channel,
						ChatID:     chatID,
						Context:    outboundCtx,
						AgentID:    outboundAgentID,
						SessionKey: outboundSessionKey,
						Scope:      outboundScope,
						Parts:      mediaParts,
					}
					if al.channelManager != nil && channel != "" {
						return al.channelManager.SendMedia(ctx, outboundMedia)
					}
					pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer pubCancel()
					return msgBus.PublishOutboundMedia(pubCtx, outboundMedia)
				}
				outboundMessage := bus.OutboundMessage{
					Channel:          channel,
					ChatID:           chatID,
					Context:          outboundCtx,
					AgentID:          outboundAgentID,
					SessionKey:       outboundSessionKey,
					Scope:            outboundScope,
					Content:          content,
					ReplyToMessageID: replyToMessageID,
				}
				if al.channelManager != nil && channel != "" {
					return al.channelManager.SendMessage(ctx, outboundMessage)
				}
				pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer pubCancel()
				return msgBus.PublishOutbound(pubCtx, outboundMessage)
			})
			agent.Tools.Register(messageTool)
		}
		if cfg.Tools.IsToolEnabled("reaction") {
			reactionTool := tools.NewReactionTool()
			reactionTool.SetReactionCallback(func(ctx context.Context, channel, chatID, messageID string) error {
				if al.channelManager == nil {
					return fmt.Errorf("channel manager not configured")
				}
				ch, ok := al.channelManager.GetChannel(channel)
				if !ok {
					return fmt.Errorf("channel %s not found", channel)
				}
				rc, ok := ch.(channels.ReactionCapable)
				if !ok {
					return fmt.Errorf("channel %s does not support reactions", channel)
				}
				_, err := rc.ReactToMessage(ctx, chatID, messageID)
				return err
			})
			agent.Tools.Register(reactionTool)
		}

		// Send file tool (outbound media via MediaStore — store injected later by SetMediaStore)
		if cfg.Tools.IsToolEnabled("send_file") {
			sendFileTool := tools.NewSendFileTool(
				agent.Workspace,
				cfg.Agents.Defaults.RestrictToWorkspace,
				cfg.Agents.Defaults.GetMaxMediaSize(),
				nil,
				allowReadPaths,
			)
			agent.Tools.Register(sendFileTool)
		}

		if cfg.Tools.IsToolEnabled("load_image") {
			loadImageTool := tools.NewLoadImageTool(
				agent.Workspace,
				cfg.Agents.Defaults.RestrictToWorkspace,
				cfg.Agents.Defaults.GetMaxMediaSize(),
				nil,
				allowReadPaths,
			)
			agent.Tools.Register(loadImageTool)
		}

		warnOnUnknownAgentToolDeclarations(agentID, agent.Workspace, agent.Definition, agent.Tools)
	}
}
