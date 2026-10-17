package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kiry163/mole/pkg/approval"
	"github.com/kiry163/mole/pkg/config"
	runtimeevents "github.com/kiry163/mole/pkg/events"
	"github.com/kiry163/mole/pkg/logger"
)

type hookRuntime struct {
	initOnce sync.Once
	mu       sync.Mutex
	initErr  error
	degraded error
	mounted  []string
}

func (r *hookRuntime) setInitErr(err error) {
	r.mu.Lock()
	r.initErr = err
	r.mu.Unlock()
}

func (r *hookRuntime) getInitErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initErr
}

// setDegraded records that some hooks could not be mounted while the rest (in
// particular the builtin approver) stayed mounted. It is separate from initErr
// because a degraded runtime must not fail turns.
func (r *hookRuntime) setDegraded(err error) {
	r.mu.Lock()
	r.degraded = err
	r.mu.Unlock()
}

// getDegraded reports a degraded hook runtime, or nil when every configured
// hook mounted.
func (r *hookRuntime) getDegraded() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.degraded
}

func (r *hookRuntime) setMounted(names []string) {
	r.mu.Lock()
	r.mounted = append([]string(nil), names...)
	r.mu.Unlock()
}

func (r *hookRuntime) reset(al *AgentLoop) {
	r.mu.Lock()
	names := append([]string(nil), r.mounted...)
	r.mounted = nil
	r.initErr = nil
	r.degraded = nil
	r.initOnce = sync.Once{}
	r.mu.Unlock()

	for _, name := range names {
		al.UnmountHook(name)
	}
}

// BuiltinHookDeps is what an in-process builtin needs from the agent: the config
// it was mounted for, the approval store the console shares, and the runtime
// event bus to publish decisions on (EventChannel only subscribes).
type BuiltinHookDeps struct {
	Config    *config.Config
	Approvals *approval.Store
	Events    runtimeevents.Bus
}

// BuiltinHookFactory constructs an in-process hook from config.
type BuiltinHookFactory func(ctx context.Context, deps BuiltinHookDeps, spec config.BuiltinHookConfig) (any, error)

var (
	builtinHookRegistryMu sync.RWMutex
	builtinHookRegistry   = map[string]BuiltinHookFactory{}
)

// RegisterBuiltinHook registers a named in-process hook factory for config-driven mounting.
func RegisterBuiltinHook(name string, factory BuiltinHookFactory) error {
	if name == "" {
		return fmt.Errorf("builtin hook name is required")
	}
	if factory == nil {
		return fmt.Errorf("builtin hook %q factory is nil", name)
	}

	builtinHookRegistryMu.Lock()
	defer builtinHookRegistryMu.Unlock()

	if _, exists := builtinHookRegistry[name]; exists {
		return fmt.Errorf("builtin hook %q is already registered", name)
	}
	builtinHookRegistry[name] = factory
	return nil
}

func unregisterBuiltinHook(name string) {
	if name == "" {
		return
	}
	builtinHookRegistryMu.Lock()
	delete(builtinHookRegistry, name)
	builtinHookRegistryMu.Unlock()
}

func lookupBuiltinHook(name string) (BuiltinHookFactory, bool) {
	builtinHookRegistryMu.RLock()
	defer builtinHookRegistryMu.RUnlock()

	factory, ok := builtinHookRegistry[name]
	return factory, ok
}

func configureHookManagerFromConfig(hm *HookManager, cfg *config.Config) {
	if hm == nil || cfg == nil {
		return
	}
	hm.ConfigureTimeouts(
		hookTimeoutFromMS(cfg.Hooks.Defaults.ObserverTimeoutMS),
		hookTimeoutFromMS(cfg.Hooks.Defaults.InterceptorTimeoutMS),
		hookTimeoutFromMS(cfg.Hooks.Defaults.ApprovalTimeoutMS),
	)
}

func hookTimeoutFromMS(ms int) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func (al *AgentLoop) ensureHooksInitialized(ctx context.Context) error {
	if al == nil || al.cfg == nil || al.hooks == nil {
		return nil
	}

	al.hookRuntime.initOnce.Do(func() {
		al.hookRuntime.setInitErr(al.loadConfiguredHooks(ctx))
	})

	return al.hookRuntime.getInitErr()
}

func (al *AgentLoop) loadConfiguredHooks(ctx context.Context) error {
	if al == nil || al.cfg == nil || !al.cfg.Hooks.Enabled {
		return nil
	}

	mounted := make([]string, 0)
	unmountMounted := func() {
		for _, name := range mounted {
			al.UnmountHook(name)
		}
	}

	builtinNames := enabledBuiltinHookNames(al.cfg.Hooks.Builtins)
	for _, name := range builtinNames {
		spec := al.cfg.Hooks.Builtins[name]
		factory, ok := lookupBuiltinHook(name)
		if !ok {
			unmountMounted()
			return fmt.Errorf("builtin hook %q is not registered", name)
		}

		hook, factoryErr := factory(ctx, BuiltinHookDeps{Config: al.cfg, Approvals: al.approvalStore, Events: al.RuntimeEventBus()}, spec)
		if factoryErr != nil {
			unmountMounted()
			return fmt.Errorf("build builtin hook %q: %w", name, factoryErr)
		}
		if err := al.MountHook(HookRegistration{
			Name:     name,
			Priority: spec.Priority,
			Source:   HookSourceInProcess,
			Hook:     hook,
		}); err != nil {
			unmountMounted()
			return fmt.Errorf("mount builtin hook %q: %w", name, err)
		}
		mounted = append(mounted, name)
	}

	// The in-process hooks are mounted now, the approval gate among them. From
	// here a process hook that fails to build or start must not take them down:
	// disarming the gate would silently approve every later tool call. Each
	// process hook is isolated, its failure is collected and reported, and the
	// builtins stay mounted.
	var degraded []error
	processNames := enabledProcessHookNames(al.cfg.Hooks.Processes)
	for _, name := range processNames {
		spec := al.cfg.Hooks.Processes[name]
		opts, buildErr := processHookOptionsFromConfig(spec)
		if buildErr != nil {
			degraded = append(degraded, fmt.Errorf("configure process hook %q: %w", name, buildErr))
			continue
		}

		processHook, startErr := NewProcessHook(ctx, name, opts)
		if startErr != nil {
			degraded = append(degraded, fmt.Errorf("start process hook %q: %w", name, startErr))
			continue
		}
		if err := al.MountHook(HookRegistration{
			Name:     name,
			Priority: spec.Priority,
			Source:   HookSourceProcess,
			Hook:     processHook,
		}); err != nil {
			_ = processHook.Close()
			degraded = append(degraded, fmt.Errorf("mount process hook %q: %w", name, err))
			continue
		}
		mounted = append(mounted, name)
	}

	al.hookRuntime.setMounted(mounted)
	if len(degraded) > 0 {
		err := errors.Join(degraded...)
		logger.ErrorCF("agent", "configured hooks are degraded: some process hooks did not start",
			map[string]any{"error": err.Error(), "mounted": mounted})
		al.hookRuntime.setDegraded(err)
	}
	return nil
}

func enabledBuiltinHookNames(specs map[string]config.BuiltinHookConfig) []string {
	if len(specs) == 0 {
		return nil
	}

	names := make([]string, 0, len(specs))
	for name, spec := range specs {
		if spec.Enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func enabledProcessHookNames(specs map[string]config.ProcessHookConfig) []string {
	if len(specs) == 0 {
		return nil
	}

	names := make([]string, 0, len(specs))
	for name, spec := range specs {
		if spec.Enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func processHookOptionsFromConfig(spec config.ProcessHookConfig) (ProcessHookOptions, error) {
	transport := spec.Transport
	if transport == "" {
		transport = "stdio"
	}
	if transport != "stdio" {
		return ProcessHookOptions{}, fmt.Errorf("unsupported transport %q", transport)
	}
	if len(spec.Command) == 0 {
		return ProcessHookOptions{}, fmt.Errorf("command is required")
	}

	opts := ProcessHookOptions{
		Command: append([]string(nil), spec.Command...),
		Dir:     spec.Dir,
		Env:     processHookEnvFromMap(spec.Env),
	}

	observeKinds, observeEnabled, err := processHookObserveKindsFromConfig(spec.Observe)
	if err != nil {
		return ProcessHookOptions{}, err
	}
	opts.Observe = observeEnabled
	opts.ObserveKinds = observeKinds

	for _, intercept := range spec.Intercept {
		switch intercept {
		case "before_llm", "after_llm":
			opts.InterceptLLM = true
		case "before_tool", "after_tool":
			opts.InterceptTool = true
		case "approve_tool":
			opts.ApproveTool = true
		case "":
			continue
		default:
			return ProcessHookOptions{}, fmt.Errorf("unsupported intercept %q", intercept)
		}
	}

	if !opts.Observe && !opts.InterceptLLM && !opts.InterceptTool && !opts.ApproveTool {
		return ProcessHookOptions{}, fmt.Errorf("no hook modes enabled")
	}

	return opts, nil
}

func processHookEnvFromMap(envMap map[string]string) []string {
	if len(envMap) == 0 {
		return nil
	}

	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+envMap[key])
	}
	return env
}

func processHookObserveKindsFromConfig(observe []string) ([]string, bool, error) {
	if len(observe) == 0 {
		return nil, false, nil
	}

	validKinds := validHookEventKinds()
	normalized := make([]string, 0, len(observe))
	for _, kind := range observe {
		switch kind {
		case "", "*", "all":
			return nil, true, nil
		default:
			normalizedKind, ok := validKinds[kind]
			if !ok {
				return nil, false, fmt.Errorf("unsupported observe event %q", kind)
			}
			normalized = append(normalized, normalizedKind)
		}
	}

	if len(normalized) == 0 {
		return nil, false, nil
	}
	return normalized, true, nil
}

func validHookEventKinds() map[string]string {
	runtimeKinds := runtimeevents.KnownKinds()
	kinds := make(map[string]string, len(runtimeKinds)*2)
	for _, kind := range runtimeKinds {
		kinds[kind.String()] = kind.String()
	}
	kinds["turn_start"] = runtimeevents.KindAgentTurnStart.String()
	kinds["turn_end"] = runtimeevents.KindAgentTurnEnd.String()
	kinds["llm_request"] = runtimeevents.KindAgentLLMRequest.String()
	kinds["llm_delta"] = runtimeevents.KindAgentLLMDelta.String()
	kinds["llm_response"] = runtimeevents.KindAgentLLMResponse.String()
	kinds["llm_retry"] = runtimeevents.KindAgentLLMRetry.String()
	kinds["context_compress"] = runtimeevents.KindAgentContextCompress.String()
	kinds["session_summarize"] = runtimeevents.KindAgentSessionSummarize.String()
	kinds["tool_exec_start"] = runtimeevents.KindAgentToolExecStart.String()
	kinds["tool_exec_end"] = runtimeevents.KindAgentToolExecEnd.String()
	kinds["tool_exec_skipped"] = runtimeevents.KindAgentToolExecSkipped.String()
	kinds["tool_approval_requested"] = runtimeevents.KindAgentToolApprovalRequested.String()
	kinds["tool_approval_decided"] = runtimeevents.KindAgentToolApprovalDecided.String()
	kinds["steering_injected"] = runtimeevents.KindAgentSteeringInjected.String()
	kinds["follow_up_queued"] = runtimeevents.KindAgentFollowUpQueued.String()
	kinds["interrupt_received"] = runtimeevents.KindAgentInterruptReceived.String()
	kinds["subturn_spawn"] = runtimeevents.KindAgentSubTurnSpawn.String()
	kinds["subturn_end"] = runtimeevents.KindAgentSubTurnEnd.String()
	kinds["subturn_result_delivered"] = runtimeevents.KindAgentSubTurnResultDelivered.String()
	kinds["subturn_orphan"] = runtimeevents.KindAgentSubTurnOrphan.String()
	kinds["error"] = runtimeevents.KindAgentError.String()
	return kinds
}

// BuiltinHookNames lists the builtin hooks registered in this process, sorted,
// so the console can offer them instead of letting a typo through.
func BuiltinHookNames() []string {
	builtinHookRegistryMu.RLock()
	defer builtinHookRegistryMu.RUnlock()

	names := make([]string, 0, len(builtinHookRegistry))
	for name := range builtinHookRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateHooksConfig reports why cfg cannot be mounted: transports, commands,
// intercepts, observe kinds, enabled modes and the builtin registry, all via the
// same code the runtime mount uses, plus positive timeout floors (the mount
// treats a non-positive timeout as "keep the default", which a caller who typed
// 0 does not want).
//
// It exists because a hooks section that fails to mount fails every later turn
// (hookRuntime caches the init error), so the console must refuse to save one
// instead of reporting success.
func ValidateHooksConfig(cfg config.HooksConfig) []string {
	if !cfg.Enabled {
		return nil
	}

	var errs []string
	if cfg.Defaults.ObserverTimeoutMS <= 0 {
		errs = append(errs, "hooks.defaults.observer_timeout_ms must be greater than 0")
	}
	if cfg.Defaults.InterceptorTimeoutMS <= 0 {
		errs = append(errs, "hooks.defaults.interceptor_timeout_ms must be greater than 0")
	}
	if cfg.Defaults.ApprovalTimeoutMS <= 0 {
		errs = append(errs, "hooks.defaults.approval_timeout_ms must be greater than 0")
	}

	builtinNames := make([]string, 0, len(cfg.Builtins))
	for name := range cfg.Builtins {
		builtinNames = append(builtinNames, name)
	}
	sort.Strings(builtinNames)
	for _, name := range builtinNames {
		if !cfg.Builtins[name].Enabled {
			continue
		}
		if _, ok := lookupBuiltinHook(name); !ok {
			errs = append(errs, fmt.Sprintf(
				"hooks.builtins.%s: builtin hook %q is not registered in this build", name, name))
		}
	}

	processNames := make([]string, 0, len(cfg.Processes))
	for name := range cfg.Processes {
		processNames = append(processNames, name)
	}
	sort.Strings(processNames)
	for _, name := range processNames {
		spec := cfg.Processes[name]
		if !spec.Enabled {
			continue
		}
		if _, err := processHookOptionsFromConfig(spec); err != nil {
			errs = append(errs, fmt.Sprintf("hooks.processes.%s: %v", name, err))
		}
		// A process hook with a builtin's name silently replaces it (HookManager.Mount
		// overwrites by name), which would disarm the builtin without telling anyone.
		if builtin, ok := cfg.Builtins[name]; ok && builtin.Enabled {
			errs = append(errs, fmt.Sprintf("hooks.processes.%s: name is already used by an enabled builtin hook", name))
		}
	}

	return errs
}
