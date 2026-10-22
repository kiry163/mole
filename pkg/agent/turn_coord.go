// Mole - Ultra-lightweight personal AI agent

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kiry163/mole/pkg/config"
	runtimeevents "github.com/kiry163/mole/pkg/events"
	"github.com/kiry163/mole/pkg/logger"
	"github.com/kiry163/mole/pkg/providers"
)

func (al *AgentLoop) runTurn(ctx context.Context, ts *turnState, pipeline *Pipeline) (turnResult, error) {
	if ts != nil && ts.agent != nil {
		modelMu := ts.agent.modelStateMutex()
		modelMu.RLock()
		defer modelMu.RUnlock()
	}
	turnCtx, turnCancel := context.WithCancel(ctx)
	defer turnCancel()
	ts.setTurnCancel(turnCancel)

	// Inject turnState and AgentLoop into context so tools (e.g. spawn) can retrieve them.
	turnCtx = withTurnState(turnCtx, ts)

	al.registerActiveTurn(ts)
	defer al.clearActiveTurn(ts)

	if al.takePendingStop(ts.sessionKey) {
		_ = ts.requestHardAbort()
	}

	turnStatus := TurnEndStatusCompleted
	defer func() {
		attemptedSkills := ts.attemptedSkillsSnapshot()
		skillContextSnapshots := ts.skillContextSnapshotsSnapshot()
		finalSuccessfulPath := []string(nil)
		if turnStatus == TurnEndStatusCompleted {
			if latest := ts.latestSkillContextSnapshot(); len(latest) > 0 {
				finalSuccessfulPath = latest
			} else {
				finalSuccessfulPath = append([]string(nil), attemptedSkills...)
			}
		}
		al.emitEvent(
			runtimeevents.KindAgentTurnEnd,
			ts.eventMeta("runTurn", "turn.end"),
			TurnEndPayload{
				Status:                turnStatus,
				Workspace:             ts.workspace,
				Iterations:            ts.currentIteration(),
				Duration:              time.Since(ts.startedAt),
				FinalContentLen:       ts.finalContentLen(),
				UserMessage:           ts.userMessage,
				FinalContent:          ts.finalContentSnapshot(),
				ActiveSkills:          append([]string(nil), ts.activeSkills...),
				AttemptedSkills:       attemptedSkills,
				FinalSuccessfulPath:   finalSuccessfulPath,
				SkillContextSnapshots: skillContextSnapshots,
				ToolKinds:             ts.toolKindsSnapshot(),
				ToolExecutions:        ts.toolExecutionsSnapshot(),
			},
		)
	}()

	if ts.hardAbortRequested() {
		turnStatus = TurnEndStatusAborted
		return al.abortTurn(ts)
	}

	al.emitEvent(
		runtimeevents.KindAgentTurnStart,
		ts.eventMeta("runTurn", "turn.start"),
		TurnStartPayload{
			UserMessage: ts.userMessage,
			MediaCount:  len(ts.media),
		},
	)

	// SetupTurn extracts the one-time initialization phase.
	exec, err := pipeline.SetupTurn(turnCtx, ts)
	if err != nil {
		return turnResult{}, err
	}
	defer exec.closeOwnedProviders()

	// Convenience references to exec fields used throughout the turn loop.
	messages := exec.messages
	pendingMessages := exec.pendingMessages
	maxMediaSize := pipeline.Cfg.Agents.Defaults.GetMaxMediaSize()
	finalContent := exec.finalContent

	for ts.currentIteration() < ts.agent.MaxIterations || len(exec.pendingMessages) > 0 || func() bool {
		graceful, _ := ts.gracefulInterruptRequested()
		return graceful
	}() {
		if ts.hardAbortRequested() {
			turnStatus = TurnEndStatusAborted
			return al.abortTurn(ts)
		}

		iteration := ts.currentIteration() + 1
		ts.setIteration(iteration)
		ts.setPhase(TurnPhaseRunning)

		if iteration > 1 {
			// For subsequent iterations, read from exec.pendingMessages which
			// is where ExecuteTools (or initial poll) deposits steering.
			// We do NOT call dequeueSteeringMessagesForScope here because
			// steering was already consumed from al.steering by ExecuteTools.
			if len(exec.pendingMessages) > 0 {
				pendingMessages = append(pendingMessages, exec.pendingMessages...)
				exec.pendingMessages = nil
			}
		} else if !ts.opts.SkipInitialSteeringPoll {
			if steerMsgs := al.dequeueSteeringMessagesForScopeWithFallback(ts.sessionKey); len(steerMsgs) > 0 {
				pendingMessages = append(pendingMessages, steerMsgs...)
			}
		}

		// Inject pending steering messages
		if len(pendingMessages) > 0 {
			resolvedPending := resolveMediaRefs(pendingMessages, al.mediaStore, maxMediaSize, 0)
			totalContentLen := 0
			for i, pm := range pendingMessages {
				messages = append(messages, resolvedPending[i])
				totalContentLen += len(pm.Content)
				ts.agent.Sessions.AddFullMessage(ts.sessionKey, pm)
				ts.recordPersistedMessage(pm)
				ts.ingestMessage(turnCtx, al, pm)
				logger.InfoCF("agent", "Injected steering message into context",
					map[string]any{
						"agent_id":    ts.agent.ID,
						"iteration":   iteration,
						"content_len": len(pm.Content),
						"media_count": len(pm.Media),
					})
			}
			al.emitEvent(
				runtimeevents.KindAgentSteeringInjected,
				ts.eventMeta("runTurn", "turn.steering.injected"),
				SteeringInjectedPayload{
					Count:           len(pendingMessages),
					TotalContentLen: totalContentLen,
				},
			)
			// Clear exec.pendingMessages after injection so InitialSteeringMessages
			// are not re-injected on subsequent iterations (Issue 2 fix).
			exec.pendingMessages = nil
		}
		// Always sync messages into exec.messages so CallLLM sees the updated state
		exec.messages = messages

		logger.DebugCF("agent", "LLM iteration",
			map[string]any{
				"agent_id":  ts.agent.ID,
				"iteration": iteration,
				"max":       ts.agent.MaxIterations,
			})

		// Execute LLM call via Pipeline
		ts.setPhase(TurnPhaseRunning)
		ctrl, callErr := pipeline.CallLLM(ctx, turnCtx, ts, exec, iteration)
		if callErr != nil {
			turnStatus = TurnEndStatusError
			return turnResult{}, callErr
		}
		messages = exec.messages
		pendingMessages = exec.pendingMessages
		finalContent = exec.finalContent

		switch ctrl {
		case ControlContinue:
			continue
		case ControlBreak:
			// Hard abort: delegate to abortTurn (sets TurnEndStatusAborted)
			if exec.abortedByHardAbort {
				turnStatus = TurnEndStatusAborted
				return al.abortTurn(ts)
			}
			// Ensure empty response falls back to DefaultResponse
			if finalContent == "" {
				finalContent = ts.opts.DefaultResponse
			}
			result, finalizeErr := pipeline.Finalize(ctx, turnCtx, ts, exec, turnStatus, finalContent)
			if finalizeErr != nil {
				turnStatus = TurnEndStatusError
			}
			return result, finalizeErr
		case ControlToolLoop:
			// Execute tools via Pipeline
			toolCtrl := pipeline.ExecuteTools(ctx, turnCtx, ts, exec, iteration)
			switch toolCtrl {
			case ToolControlContinue:
				// Re-read exec.messages since ExecuteTools may have updated it
				// (added tool results/skipped messages) before returning ControlContinue
				messages = exec.messages
				continue
			case ToolControlBreak:
				// Hard abort: delegate to abortTurn (sets TurnEndStatusAborted)
				if exec.abortedByHardAbort {
					turnStatus = TurnEndStatusAborted
					return al.abortTurn(ts)
				}
				// ExecuteTools returned ControlBreak:
				// - allResponsesHandled=true: finalize without DefaultResponse (exec.finalContent empty)
				// - allResponsesHandled=false: coordinator applies DefaultResponse before finalize
				if exec.allResponsesHandled {
					finalContent = ""
				}
				result, finalizeErr := pipeline.Finalize(ctx, turnCtx, ts, exec, turnStatus, finalContent)
				if finalizeErr != nil {
					turnStatus = TurnEndStatusError
				}
				return result, finalizeErr
			}
		}
	}

	if ts.hardAbortRequested() {
		turnStatus = TurnEndStatusAborted
		return al.abortTurn(ts)
	}

	if finalContent == "" {
		if ts.currentIteration() >= ts.agent.MaxIterations && ts.agent.MaxIterations > 0 {
			finalContent = toolLimitResponse
		} else {
			finalContent = ts.opts.DefaultResponse
		}
	}

	// Check hard abort before finalizing (may have been set during tool execution)
	if ts.hardAbortRequested() {
		turnStatus = TurnEndStatusAborted
		return al.abortTurn(ts)
	}

	result, err := pipeline.Finalize(ctx, turnCtx, ts, exec, turnStatus, finalContent)
	if err != nil {
		turnStatus = TurnEndStatusError
	}
	return result, err
}

func (al *AgentLoop) abortTurn(ts *turnState) (turnResult, error) {
	ts.setPhase(TurnPhaseAborted)
	if err := ts.restoreSession(ts.agent); err != nil {
		al.emitEvent(
			runtimeevents.KindAgentError,
			ts.eventMeta("abortTurn", "turn.error"),
			ErrorPayload{
				Stage:   "session_restore",
				Message: err.Error(),
			},
		)
		return turnResult{}, err
	}
	return turnResult{status: TurnEndStatusAborted}, nil
}

func (al *AgentLoop) selectCandidates(
	agent *AgentInstance,
	_ string,
	_ []providers.Message,
) (candidates []providers.FallbackCandidate, model string) {
	return agent.Candidates, resolvedCandidateModel(agent.Candidates, agent.Model)
}

func (al *AgentLoop) resolveContextManager() *sessionContextManager {
	return &sessionContextManager{al: al}
}

func (al *AgentLoop) askSideQuestion(
	ctx context.Context,
	agent *AgentInstance,
	opts *processOptions,
	question string,
) (string, error) {
	if agent == nil {
		return "", fmt.Errorf("askSideQuestion: no agent available for /btw")
	}
	modelMu := agent.modelStateMutex()
	modelMu.RLock()
	defer modelMu.RUnlock()

	question = strings.TrimSpace(question)
	if question == "" {
		return "", fmt.Errorf("askSideQuestion: %w", fmt.Errorf("Usage: /btw <question>"))
	}

	if opts != nil {
		normalizeProcessOptionsInPlace(opts)
	}

	var media []string
	var channel, chatID, senderID, senderDisplayName string
	if opts != nil {
		media = opts.Media
		channel = opts.Channel
		chatID = opts.ChatID
		senderID = opts.SenderID
		senderDisplayName = opts.SenderDisplayName
	}

	// Build messages with context but WITHOUT adding to session history
	var history []providers.Message
	var summary string
	if opts != nil {
		if resp, err := al.contextManager.Assemble(ctx, &AssembleRequest{
			SessionKey: opts.SessionKey,
			Budget:     agent.ContextWindow,
			MaxTokens:  agent.MaxTokens,
		}); err == nil && resp != nil {
			history = resp.History
			summary = resp.Summary
		}
	}

	var promptReq PromptBuildRequest
	if opts == nil {
		promptReq = PromptBuildRequest{
			History:           history,
			Summary:           summary,
			CurrentMessage:    question,
			Media:             append([]string(nil), media...),
			Channel:           channel,
			ChatID:            chatID,
			SenderID:          senderID,
			SenderDisplayName: senderDisplayName,
		}
	} else {
		promptReq = promptBuildRequestForProcessOptions(
			agent,
			*opts,
			history,
			summary,
			question,
			media,
		)
	}
	promptReq.SuppressToolUseRule = true
	promptReq.ToolUseFallback = false
	messages := agent.ContextBuilder.BuildMessagesFromPrompt(promptReq)

	maxMediaSize := al.GetConfig().Agents.Defaults.GetMaxMediaSize()
	currentTurnStart := len(messages)
	if strings.TrimSpace(question) != "" || len(media) > 0 {
		currentTurnStart = len(messages) - 1
	}
	messages = resolveMediaRefs(messages, al.mediaStore, maxMediaSize, currentTurnStart)

	activeCandidates, _ := al.selectCandidates(agent, question, messages)

	llmOpts := map[string]any{
		"max_tokens":       agent.MaxTokens,
		"temperature":      agent.Temperature,
		"prompt_cache_key": agent.ID + ":btw",
	}

	sideSuppressReasoning := false
	callProvider := func(
		ctx context.Context,
		candidate providers.FallbackCandidate,
		callMessages []providers.Message,
	) (*providers.LLMResponse, error) {
		baseModelName := agent.Model
		provider, providerModel, modelCfg, cleanup, err := al.isolatedSideQuestionProvider(
			agent,
			baseModelName,
			candidate,
		)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		callOpts := llmOpts
		settings := thinkingSettingsFromModelConfig(modelCfg)
		sideSuppressReasoning = shouldSuppressReasoningFor(settings)
		if _, exists := callOpts["thinking_level"]; !exists {
			if settings.configured {
				callOpts = shallowCloneLLMOptions(llmOpts)
				applyThinkingOption(callOpts, provider, settings, false, agent.ID)
			}
		}
		return provider.Chat(ctx, callMessages, nil, providerModel, callOpts)
	}

	turnCtx := newTurnContext(nil, nil, nil)
	if opts != nil {
		turnCtx = newTurnContext(opts.Dispatch.InboundContext, opts.Dispatch.RouteResult, opts.Dispatch.SessionScope)
	}
	callSideLLM := func(callMessages []providers.Message) (*providers.LLMResponse, error) {
		if len(activeCandidates) > 1 && al.fallback != nil {
			fbResult, err := al.fallback.ExecuteCandidate(
				ctx,
				activeCandidates,
				func(ctx context.Context, candidate providers.FallbackCandidate) (*providers.LLMResponse, error) {
					return callProvider(ctx, candidate, callMessages)
				},
			)
			if err != nil {
				return nil, err
			}
			return fbResult.Response, nil
		}

		var candidate providers.FallbackCandidate
		if len(activeCandidates) > 0 {
			candidate = activeCandidates[0]
		}
		return callProvider(ctx, candidate, callMessages)
	}

	// Retry without media if vision is unsupported
	// Note: Vision retry is only applied to the initial call. If fallback chain
	// is used, vision errors from fallback providers will not trigger retry.
	var resp *providers.LLMResponse
	var err error
	resp, err = callSideLLM(messages)
	if err != nil && hasMediaRefs(messages) && isVisionUnsupportedError(err) {
		al.emitEvent(
			runtimeevents.KindAgentLLMRetry,
			EventMeta{
				Source:      "askSideQuestion",
				TracePath:   "turn.llm.retry",
				turnContext: cloneTurnContext(turnCtx),
			},
			LLMRetryPayload{
				Attempt:    1,
				MaxRetries: 1,
				Reason:     "vision_unsupported",
				Error:      err.Error(),
				Backoff:    0,
			},
		)
		messagesWithoutMedia := stripMessageMedia(messages)
		resp, err = callSideLLM(messagesWithoutMedia)
	}
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}

	if sideSuppressReasoning {
		resp.Reasoning = ""
		resp.ReasoningContent = ""
		resp.ReasoningDetails = nil
	}

	return sideQuestionResponseContent(resp), nil
}

func (al *AgentLoop) isolatedSideQuestionProvider(
	agent *AgentInstance,
	baseModelName string,
	candidate providers.FallbackCandidate,
) (providers.LLMProvider, string, *config.ModelConfig, func(), error) {
	if agent == nil {
		return nil, "", nil, func() {}, fmt.Errorf("isolatedSideQuestionProvider: no agent available for /btw")
	}

	modelCfg, err := al.sideQuestionModelConfig(agent, baseModelName, candidate)
	if err != nil {
		return nil, "", nil, func() {}, fmt.Errorf("isolatedSideQuestionProvider: %w", err)
	}

	factory := al.providerFactory
	if factory == nil {
		factory = providers.CreateProviderFromConfig
	}
	provider, modelID, err := factory(modelCfg)
	if err != nil {
		return nil, "", nil, func() {}, fmt.Errorf("isolatedSideQuestionProvider: %w", err)
	}

	cleanup := func() {
		closeProviderIfStateful(provider)
	}
	return provider, modelID, modelCfg, cleanup, nil
}

func (al *AgentLoop) sideQuestionModelConfig(
	agent *AgentInstance,
	baseModelName string,
	candidate providers.FallbackCandidate,
) (*config.ModelConfig, error) {
	if agent == nil {
		return nil, fmt.Errorf("sideQuestionModelConfig: no agent available for /btw")
	}
	if candidate.ConfigIndex > 0 || candidate.ConfigKey != "" {
		if modelCfg, err := resolvedCandidateModelConfig(
			al.GetConfig(),
			candidate,
		); err == nil {
			return modelCfg, nil
		}
	}

	if name := modelAliasFromCandidateIdentityKey(candidate.IdentityKey); name != "" {
		if modelCfg, err := resolvedRuntimeModelConfig(al.GetConfig(), name); err == nil {
			return modelCfg, nil
		}
	}

	baseModelName = strings.TrimSpace(baseModelName)
	if baseModelName != "" {
		if modelCfg, err := resolvedRuntimeModelConfig(al.GetConfig(), baseModelName); err == nil {
			return modelCfg, nil
		}
	}

	// Limited mode (no model_list) still allows /btw to use the injected
	// provider with the active model identifier.
	modelName := baseModelName
	if modelName == "" {
		modelName = strings.TrimSpace(candidate.Model)
	}
	if modelName == "" {
		return nil, fmt.Errorf("sideQuestionModelConfig: no configured model for /btw")
	}
	provider := strings.TrimSpace(candidate.Provider)
	if provider == "" {
		provider = "openai"
	}
	return &config.ModelConfig{
		ModelName: modelName,
		Provider:  provider,
		Model:     modelName,
	}, nil
}
