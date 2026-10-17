package gateway

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kiry163/mole/pkg/agent"
	"github.com/kiry163/mole/pkg/approval"
	"github.com/kiry163/mole/pkg/bus"
	"github.com/kiry163/mole/pkg/channels"
	_ "github.com/kiry163/mole/pkg/channels/feishu"
	_ "github.com/kiry163/mole/pkg/channels/qq"
	_ "github.com/kiry163/mole/pkg/channels/telegram"
	_ "github.com/kiry163/mole/pkg/channels/webchat"
	_ "github.com/kiry163/mole/pkg/channels/weixin"
	"github.com/kiry163/mole/pkg/config"
	runtimeevents "github.com/kiry163/mole/pkg/events"
	"github.com/kiry163/mole/pkg/health"
	"github.com/kiry163/mole/pkg/logger"
	"github.com/kiry163/mole/pkg/media"
	"github.com/kiry163/mole/pkg/netbind"
	"github.com/kiry163/mole/pkg/pid"
	"github.com/kiry163/mole/pkg/providers"
	"github.com/kiry163/mole/pkg/utils"
)

const (
	serviceShutdownTimeout  = 30 * time.Second
	providerReloadTimeout   = 30 * time.Second
	gracefulShutdownTimeout = 15 * time.Second

	logPath   = "logs"
	panicFile = "gateway_panic.log"
	logFile   = "gateway.log"
)

type services struct {
	MediaStore       media.MediaStore
	ChannelManager   *channels.Manager
	HealthServer     *health.Server
	VoiceAgentCancel context.CancelFunc
	manualReloadChan chan struct{}
	reloading        atomic.Bool
	authToken        string
}

type startupBlockedProvider struct {
	reason string
}

func (p *startupBlockedProvider) Chat(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	return nil, fmt.Errorf("%s", p.reason)
}

func (p *startupBlockedProvider) GetDefaultModel() string {
	return ""
}

// Options configures a single gateway run.
type Options struct {
	Debug      bool
	NoTruncate bool
	HomePath   string
	ConfigPath string
	Host       string                  // overrides gateway.host; empty keeps the config/env value
	Port       int                     // overrides gateway.port; 0 keeps the configured port
	Mount      func(*RuntimeEnv) error // nil starts the gateway without extra HTTP handlers

	// NotifyStarted is called once: nil after all services are ready, or the startup error.
	NotifyStarted func(error)
}

// RuntimeEnv exposes the running gateway to an in-process HTTP mount.
type RuntimeEnv struct {
	Manager       *channels.Manager
	AgentLoop     *agent.AgentLoop
	Config        *config.Config
	Address       string // actual bind address (host:port)
	RequestReload func()
	// Approvals returns the process-wide approval store. It is a provider
	// because the gateway hands it to the console at mount time.
	Approvals func() *approval.Store
}

// mount hands the runtime environment to the configured mount function.
// A nil Mount is a no-op so headless runs stay unchanged.
func (o Options) mount(env *RuntimeEnv) error {
	if o.Mount == nil {
		return nil
	}
	return o.Mount(env)
}

// Run starts the gateway runtime using the configuration loaded from opts.ConfigPath.
// It blocks until ctx is cancelled, then shuts the runtime down.
func Run(ctx context.Context, opts Options) (runErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	notified := false
	notifyStarted := func(err error) {
		if notified || opts.NotifyStarted == nil {
			return
		}
		notified = true
		opts.NotifyStarted(err)
	}
	defer func() { notifyStarted(runErr) }()

	debug := opts.Debug
	homePath := opts.HomePath
	configPath := opts.ConfigPath

	if opts.NoTruncate {
		utils.SetDisableTruncation(true)
		logger.Info("String truncation is globally disabled via 'no-truncate' flag")
	}

	startedAt := time.Now()
	panicPath := filepath.Join(homePath, logPath, panicFile)
	panicFunc, err := logger.InitPanic(panicPath)
	if err != nil {
		return fmt.Errorf("error initializing panic log: %w", err)
	}
	defer panicFunc()

	if err = logger.EnableFileLogging(filepath.Join(homePath, logPath, logFile)); err != nil {
		logger.Fatal(fmt.Sprintf("error enabling file logging: %v", err))
	}
	defer logger.DisableFileLogging()

	if debug {
		logger.SetLevel(logger.DEBUG)
	} else {
		logger.SetLevelFromString(config.ResolveGatewayLogLevel(configPath))
	}
	defer func() {
		if runErr != nil {
			logger.ErrorCF("gateway", "Gateway startup failed", map[string]any{
				"config_path": configPath,
				"error":       runErr.Error(),
				"home_path":   homePath,
				"debug":       debug,
			})
		}
	}()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	if err = applyBindOverrides(cfg, opts); err != nil {
		return err
	}

	if err = preCheckConfig(cfg); err != nil {
		return fmt.Errorf("config pre-check failed: %w", err)
	}

	// Debug mode permanently overrides the config log level to DEBUG.
	if debug {
		fmt.Println("🔍 Debug mode enabled")
	} else {
		effectiveLogLevel := config.EffectiveGatewayLogLevel(cfg)
		logger.SetLevelFromString(effectiveLogLevel)
		logger.Infof("Log level set to %q", effectiveLogLevel)
	}

	bindPlan, listenResult, err := openGatewayListeners(cfg.Gateway.Host, cfg.Gateway.Port)
	if err != nil {
		return fmt.Errorf("error opening gateway listeners: %w", err)
	}

	// Enforce singleton: write PID file with generated token.
	pidData, err := pid.WritePidFile(homePath, bindPlan.ProbeHost, cfg.Gateway.Port)
	if err != nil {
		logger.Warnf("write pid file failed: %v", err)
		for _, ln := range listenResult.Listeners {
			_ = ln.Close()
		}
		return fmt.Errorf("singleton check failed: %w", err)
	}
	defer pid.RemovePidFile(homePath)
	closeListeners := true
	defer func() {
		if !closeListeners {
			return
		}
		for _, ln := range listenResult.Listeners {
			_ = ln.Close()
		}
	}()

	provider, modelID, err := createStartupProvider(cfg)
	if err != nil {
		return fmt.Errorf("error creating provider: %w", err)
	}

	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)
	// One approval store per process: the builtin approver waits on it and the
	// console answers it. It must outlive config reloads, which remount hooks
	// while a human may be looking at a pending request, so only the final
	// shutdown closes it (never the reload path).
	approvalStore := approval.NewStore()
	agentLoop.SetApprovalStore(approvalStore)
	msgBus.SetEventPublisher(agentLoop.RuntimeEventBus())
	publishGatewayEvent(agentLoop, runtimeevents.KindGatewayStart, startedAt, nil)

	fmt.Println("\n📦 Agent Status:")
	startupStatus := collectGatewayStartupStatus(agentLoop.GetStartupInfo())
	fmt.Printf("  • Tools: %d loaded\n", startupStatus.toolsCount)
	fmt.Printf("  • Skills: %d/%d available\n", startupStatus.skillsAvailable, startupStatus.skillsTotal)

	logger.InfoCF("agent", "Agent initialized", startupStatus.logFields)

	runningServices, err := setupAndStartServices(cfg, agentLoop, msgBus, approvalStore, pidData.Token, listenResult, opts)
	if err != nil {
		return err
	}
	// All services (channels + shared HTTP server) are up; mark the health
	// server ready so GET /ready reports "ready". The health endpoints are
	// mounted on the shared gateway mux, so Health.Server.Start() (which would
	// otherwise set this) is never called — we flip the flag explicitly here.
	runningServices.HealthServer.SetReady(true)
	publishGatewayEvent(agentLoop, runtimeevents.KindGatewayReady, startedAt, nil)
	closeListeners = false
	notifyStarted(nil)

	for _, bindHost := range listenResult.BindHosts {
		fmt.Printf("✓ Gateway started on %s\n", net.JoinHostPort(bindHost, strconv.Itoa(cfg.Gateway.Port)))
	}
	fmt.Println("Press Ctrl+C to stop")

	go agentLoop.Run(ctx)

	var configReloadChan <-chan *config.Config
	stopWatch := func() {}
	if cfg.Gateway.HotReload {
		configReloadChan, stopWatch = setupConfigWatcherPolling(configPath, debug)
		logger.Info("Config hot reload enabled")
	}
	defer stopWatch()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down...")
			shutdownGateway(runningServices, agentLoop, provider, msgBus, true)
			// Close only here, never on the reload path: a reload remounts the
			// hooks while a human may still be looking at a pending request, so
			// the store and its waiters must survive it. The final shutdown is
			// where every waiter gets resolved.
			approvalStore.Close()
			return nil
		case newCfg := <-configReloadChan:
			if !runningServices.reloading.CompareAndSwap(false, true) {
				logger.Warn("Config reload skipped: another reload is in progress")
				continue
			}
			err := executeReload(ctx, agentLoop, newCfg, &provider, runningServices, debug)
			if err != nil {
				logger.Errorf("Config reload failed: %v", err)
			}
		case <-runningServices.manualReloadChan:
			logger.Info("Manual reload triggered via /reload endpoint")
			newCfg, err := config.LoadConfig(configPath)
			if err != nil {
				logger.Errorf("Error loading config for manual reload: %v", err)
				runningServices.reloading.Store(false)
				continue
			}
			if err = newCfg.ValidateModelList(); err != nil {
				logger.Errorf("Config validation failed: %v", err)
				runningServices.reloading.Store(false)
				continue
			}
			err = executeReload(ctx, agentLoop, newCfg, &provider, runningServices, debug)
			if err != nil {
				logger.Errorf("Manual reload failed: %v", err)
			} else {
				logger.Info("Manual reload completed successfully")
			}
		}
	}
}

// newReloadTrigger builds the shared manual reload trigger used by the /reload
// endpoint, the agent loop and RuntimeEnv.RequestReload. It must be called once
// per running gateway.
func (s *services) newReloadTrigger() func() error {
	s.manualReloadChan = make(chan struct{}, 1)
	return func() error {
		if !s.reloading.CompareAndSwap(false, true) {
			return fmt.Errorf("reload already in progress")
		}
		select {
		case s.manualReloadChan <- struct{}{}:
			return nil
		default:
			// Should not happen, but reset flag if channel is full
			s.reloading.Store(false)
			return fmt.Errorf("reload already queued")
		}
	}
}

// applyBindOverrides applies the Options host/port overrides on top of the loaded
// configuration. An empty host or a zero port keeps the configured value, so the
// override takes precedence over both the config file and the environment.
func applyBindOverrides(cfg *config.Config, opts Options) error {
	if opts.Host != "" {
		normalized, err := netbind.NormalizeHostInput(opts.Host)
		if err != nil {
			return fmt.Errorf("invalid gateway host override: %w", err)
		}
		cfg.Gateway.Host = normalized
	}
	if opts.Port != 0 {
		cfg.Gateway.Port = opts.Port
	}
	return nil
}

func preCheckConfig(cfg *config.Config) error {
	if cfg.Gateway.Port <= 0 || cfg.Gateway.Port > 65535 {
		return fmt.Errorf("invalid gateway port: %d, port must be between 1 and 65535", cfg.Gateway.Port)
	}
	return nil
}

type gatewayStartupStatus struct {
	toolsCount      int
	skillsAvailable int
	skillsTotal     int
	logFields       map[string]any
}

func collectGatewayStartupStatus(startupInfo map[string]any) gatewayStartupStatus {
	status := gatewayStartupStatus{logFields: map[string]any{}}

	if toolsInfo, ok := startupInfo["tools"].(map[string]any); ok {
		if count, ok := startupInfoInt(toolsInfo["count"]); ok {
			status.toolsCount = count
			status.logFields["tools_count"] = count
		}
	}

	if skillsInfo, ok := startupInfo["skills"].(map[string]any); ok {
		if total, ok := startupInfoInt(skillsInfo["total"]); ok {
			status.skillsTotal = total
			status.logFields["skills_total"] = total
		}
		if available, ok := startupInfoInt(skillsInfo["available"]); ok {
			status.skillsAvailable = available
			status.logFields["skills_available"] = available
		}
	}

	return status
}

func startupInfoInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func executeReload(
	ctx context.Context,
	agentLoop *agent.AgentLoop,
	newCfg *config.Config,
	provider *providers.LLMProvider,
	runningServices *services,
	debug bool,
) (err error) {
	startedAt := time.Now()
	publishGatewayEvent(agentLoop, runtimeevents.KindGatewayReloadStarted, startedAt, nil)
	defer runningServices.reloading.Store(false)
	defer func() {
		if err != nil {
			publishGatewayEvent(agentLoop, runtimeevents.KindGatewayReloadFailed, startedAt, err)
			return
		}
		publishGatewayEvent(agentLoop, runtimeevents.KindGatewayReloadCompleted, startedAt, nil)
	}()

	err = handleConfigReload(ctx, agentLoop, newCfg, provider, runningServices, debug)
	return err
}

// createStartupProvider builds the provider for the configured default model.
// Without a default model the gateway starts in limited mode: the console serves
// its first-run setup page instead of the process exiting.
func createStartupProvider(cfg *config.Config) (providers.LLMProvider, string, error) {
	modelName := cfg.Agents.Defaults.GetModelName()
	if modelName == "" {
		reason := "no default model configured; gateway started in limited mode"
		fmt.Printf("⚠ Warning: %s\n", reason)
		logger.WarnCF("gateway", "Gateway started without default model", map[string]any{
			"limited_mode": true,
		})
		return &startupBlockedProvider{reason: reason}, "", nil
	}

	provider, modelID, err := providers.CreateProvider(cfg)
	if err != nil {
		return nil, "", err
	}
	return provider, modelID, nil
}

func setupAndStartServices(
	cfg *config.Config,
	agentLoop *agent.AgentLoop,
	msgBus *bus.MessageBus,
	approvalStore *approval.Store,
	authToken string,
	listenResult netbind.OpenResult,
	opts Options,
) (*services, error) {
	runningServices := &services{}

	var err error
	runningServices.MediaStore = media.NewFileMediaStoreWithCleanup(media.MediaCleanerConfig{
		Enabled:  cfg.Tools.MediaCleanup.Enabled,
		MaxAge:   time.Duration(cfg.Tools.MediaCleanup.MaxAge) * time.Minute,
		Interval: time.Duration(cfg.Tools.MediaCleanup.Interval) * time.Minute,
	})
	if fms, ok := runningServices.MediaStore.(*media.FileMediaStore); ok {
		fms.Start()
	}

	runningServices.ChannelManager, err = channels.NewManager(
		cfg,
		msgBus,
		runningServices.MediaStore,
		channels.WithRuntimeEvents(agentLoop.RuntimeEventBus()),
	)
	if err != nil {
		if fms, ok := runningServices.MediaStore.(*media.FileMediaStore); ok {
			fms.Stop()
		}
		return nil, fmt.Errorf("error creating channel manager: %w", err)
	}

	agentLoop.SetChannelManager(runningServices.ChannelManager)
	agentLoop.SetMediaStore(runningServices.MediaStore)

	enabledChannels := runningServices.ChannelManager.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("✓ Channels enabled: %s\n", enabledChannels)
	} else {
		fmt.Println("⚠ Warning: No channels enabled")
	}

	runningServices.authToken = authToken
	runningServices.HealthServer = health.NewServer(listenResult.ProbeHost, cfg.Gateway.Port, authToken)

	var listenAddr string
	if len(listenResult.Listeners) > 0 {
		listenAddr = listenResult.Listeners[0].Addr().String()
	} else {
		listenAddr = net.JoinHostPort(listenResult.ProbeHost, strconv.Itoa(cfg.Gateway.Port))
	}
	runningServices.ChannelManager.SetupHTTPServerListeners(
		listenResult.Listeners,
		listenAddr,
		runningServices.HealthServer,
	)

	// The manual reload trigger is created before the channels start so that an
	// in-process mount can already request a reload through RuntimeEnv.
	reloadTrigger := runningServices.newReloadTrigger()
	runningServices.HealthServer.SetReloadFunc(reloadTrigger)
	agentLoop.SetReloadFunc(reloadTrigger)

	// Mount extra HTTP handlers on the shared gateway mux before the channels
	// start serving: SetupHTTPServerListeners replaces the mux, so mounting any
	// earlier would be silently dropped.
	if err = opts.mount(&RuntimeEnv{
		Manager:       runningServices.ChannelManager,
		AgentLoop:     agentLoop,
		Config:        cfg,
		Address:       listenAddr,
		RequestReload: func() { _ = reloadTrigger() },
		Approvals:     func() *approval.Store { return approvalStore },
	}); err != nil {
		return nil, fmt.Errorf("error mounting HTTP handlers: %w", err)
	}

	if err = runningServices.ChannelManager.StartAll(context.Background()); err != nil {
		return nil, fmt.Errorf("error starting channels: %w", err)
	}

	healthAddr := net.JoinHostPort(listenResult.ProbeHost, strconv.Itoa(cfg.Gateway.Port))
	fmt.Printf(
		"✓ Health endpoints available at http://%s/health, /ready and /reload (POST)\n",
		healthAddr,
	)

	return runningServices, nil
}

func stopAndCleanupServices(runningServices *services, shutdownTimeout time.Duration, isReload bool) {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// reload should not stop channel manager
	if !isReload && runningServices.ChannelManager != nil {
		runningServices.ChannelManager.StopAll(shutdownCtx)
	}
	if runningServices.VoiceAgentCancel != nil {
		runningServices.VoiceAgentCancel()
	}
	if runningServices.MediaStore != nil {
		if fms, ok := runningServices.MediaStore.(*media.FileMediaStore); ok {
			fms.Stop()
		}
	}
}

func shutdownGateway(
	runningServices *services,
	agentLoop *agent.AgentLoop,
	provider providers.LLMProvider,
	msgBus *bus.MessageBus,
	fullShutdown bool,
) {
	publishGatewayEvent(agentLoop, runtimeevents.KindGatewayShutdown, time.Time{}, nil)

	if cp, ok := provider.(providers.StatefulProvider); ok && fullShutdown {
		cp.Close()
	}

	stopAndCleanupServices(runningServices, gracefulShutdownTimeout, false)

	if fullShutdown && msgBus != nil {
		msgBus.Close()
	}

	agentLoop.Stop()
	agentLoop.Close()

	logger.Info("✓ Gateway stopped")
}

func handleConfigReload(
	ctx context.Context,
	al *agent.AgentLoop,
	newCfg *config.Config,
	providerRef *providers.LLMProvider,
	runningServices *services,
	debug bool,
) error {
	logger.Info("🔄 Config file changed, reloading...")

	newModel := newCfg.Agents.Defaults.ModelName

	logger.Infof(" New model is '%s', recreating provider...", newModel)

	logger.Info("  Stopping all services...")
	stopAndCleanupServices(runningServices, serviceShutdownTimeout, true)

	newProvider, newModelID, err := createStartupProvider(newCfg)
	if err != nil {
		logger.Errorf("  ⚠ Error creating new provider: %v", err)
		logger.Warn("  Attempting to restart services with old provider and config...")
		if restartErr := restartServices(al, runningServices); restartErr != nil {
			logger.Errorf("  ⚠ Failed to restart services: %v", restartErr)
		}
		return fmt.Errorf("error creating new provider: %w", err)
	}

	if newModelID != "" {
		newCfg.Agents.Defaults.ModelName = newModelID
	}

	reloadCtx, reloadCancel := context.WithTimeout(context.Background(), providerReloadTimeout)
	defer reloadCancel()

	if err := al.ReloadProviderAndConfig(reloadCtx, newProvider, newCfg); err != nil {
		logger.Errorf("  ⚠ Error reloading agent loop: %v", err)
		if cp, ok := newProvider.(providers.StatefulProvider); ok {
			cp.Close()
		}
		logger.Warn("  Attempting to restart services with old provider and config...")
		if restartErr := restartServices(al, runningServices); restartErr != nil {
			logger.Errorf("  ⚠ Failed to restart services: %v", restartErr)
		}
		return fmt.Errorf("error reloading agent loop: %w", err)
	}

	*providerRef = newProvider

	logger.Info("  Restarting all services with new configuration...")
	if err := restartServices(al, runningServices); err != nil {
		logger.Errorf("  ⚠ Error restarting services: %v", err)
		return fmt.Errorf("error restarting services: %w", err)
	}

	logger.Info("  ✓ Provider, configuration, and services reloaded successfully (thread-safe)")

	// Debug mode permanently overrides the config log level to DEBUG.
	if !debug {
		// Update log level last so that reload-related info/warn logs above are not suppressed.
		effectiveLogLevel := config.EffectiveGatewayLogLevel(newCfg)
		logger.SetLevelFromString(effectiveLogLevel)
		logger.Infof("Log level changing from current to %q", effectiveLogLevel)
	}

	return nil
}

func restartServices(
	al *agent.AgentLoop,
	runningServices *services,
) error {
	cfg := al.GetConfig()

	var err error

	runningServices.MediaStore = media.NewFileMediaStoreWithCleanup(media.MediaCleanerConfig{
		Enabled:  cfg.Tools.MediaCleanup.Enabled,
		MaxAge:   time.Duration(cfg.Tools.MediaCleanup.MaxAge) * time.Minute,
		Interval: time.Duration(cfg.Tools.MediaCleanup.Interval) * time.Minute,
	})
	if fms, ok := runningServices.MediaStore.(*media.FileMediaStore); ok {
		fms.Start()
	}
	if runningServices.ChannelManager != nil {
		runningServices.ChannelManager.SetMediaStore(runningServices.MediaStore)
	}
	al.SetMediaStore(runningServices.MediaStore)

	al.SetChannelManager(runningServices.ChannelManager)

	if err = runningServices.ChannelManager.Reload(context.Background(), cfg); err != nil {
		return fmt.Errorf("error reload channels: %w", err)
	}
	fmt.Println("  ✓ Channels restarted.")

	enabledChannels := runningServices.ChannelManager.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("  ✓ Channels enabled: %s\n", enabledChannels)
	} else {
		fmt.Println("  ⚠ Warning: No channels enabled")
	}

	// NOTE: PID file is written once at startup and not updated on reload.
	// Changing the gateway listen address requires a full restart.

	return nil
}

func setupConfigWatcherPolling(configPath string, debug bool) (chan *config.Config, func()) {
	configChan := make(chan *config.Config, 1)
	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		lastModTime := getFileModTime(configPath)
		lastSize := getFileSize(configPath)

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				currentModTime := getFileModTime(configPath)
				currentSize := getFileSize(configPath)

				if currentModTime.After(lastModTime) || currentSize != lastSize {
					if debug {
						logger.Debugf("🔍 Config file change detected")
					}

					time.Sleep(500 * time.Millisecond)

					lastModTime = currentModTime
					lastSize = currentSize

					newCfg, err := config.LoadConfig(configPath)
					if err != nil {
						logger.Errorf("⚠ Error loading new config: %v", err)
						logger.Warn("  Using previous valid config")
						continue
					}

					if err := newCfg.ValidateModelList(); err != nil {
						logger.Errorf("  ⚠ New config validation failed: %v", err)
						logger.Warn("  Using previous valid config")
						continue
					}

					logger.Info("✓ Config file validated and loaded")

					select {
					case configChan <- newCfg:
					default:
						logger.Warn("⚠ Previous config reload still in progress, skipping")
					}
				}
			case <-stop:
				return
			}
		}
	}()

	stopFunc := func() {
		close(stop)
		wg.Wait()
	}

	return configChan, stopFunc
}

func getFileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func getFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
