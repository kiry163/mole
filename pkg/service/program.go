// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"

	svc "github.com/kardianos/service"

	"github.com/kiry163/mole/pkg/gateway"
	"github.com/kiry163/mole/pkg/logger"
)

const (
	defaultStartTimeout = 30 * time.Second
	defaultStopTimeout  = 45 * time.Second
)

// Program adapts the gateway to kardianos/service. Start loads the instance
// environment, launches the gateway, and waits for it to report readiness so
// the service manager only sees a running service once the HTTP listener is up.
type Program struct {
	Options gateway.Options

	run func(context.Context, gateway.Options) error
	// notify forwards gateway readiness to started. It is installed into
	// Options.NotifyStarted by NewProgram and never replaced afterwards.
	notify func(error)
	// started buffers the one readiness report. Sending must not block when
	// Start has already returned (for example after a timeout).
	started chan error
	done    chan struct{}

	mu       sync.Mutex
	cancel   context.CancelFunc
	err      error
	stopping bool

	startTimeout time.Duration
	stopTimeout  time.Duration
	interactive  func() bool
}

// NewProgram returns a Program that runs the gateway. Tests replace run.
func NewProgram(opts gateway.Options) *Program {
	p := &Program{
		Options:      opts,
		run:          gateway.Run,
		started:      make(chan error, 1),
		done:         make(chan struct{}),
		startTimeout: defaultStartTimeout,
		stopTimeout:  defaultStopTimeout,
		interactive:  svc.Interactive,
	}
	p.notify = func(err error) {
		select {
		case p.started <- err:
		default:
		}
	}
	p.Options.NotifyStarted = p.notify
	return p
}

// Start implements svc.Interface. It returns once the gateway is ready, or with
// the startup error/timeout. The gateway keeps running in the background.
func (p *Program) Start(s svc.Service) error {
	if err := LoadEnv(p.Options.ConfigPath); err != nil {
		return err
	}
	applyTimezone()

	ctx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.cancel = cancel
	p.mu.Unlock()

	go func() {
		err := p.run(ctx, p.Options)
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(p.done)
	}()

	timer := time.NewTimer(p.startTimeout)
	defer timer.Stop()
	select {
	case err := <-p.started:
		if err != nil {
			cancel()
			return err
		}
	case <-timer.C:
		cancel()
		return fmt.Errorf("gateway did not become ready within %s", p.startTimeout)
	case <-p.done:
		// The gateway returned before Start observed the report. Drain the
		// buffered readiness result: gateway.Run reports through a defer, so a
		// completed run always has one.
		select {
		case err := <-p.started:
			if err != nil {
				return err
			}
		default:
			return p.exitError()
		}
	}

	go p.watch(s)
	return nil
}

// Stop implements svc.Interface. It cancels the gateway context and waits for
// the run goroutine to exit.
func (p *Program) Stop(_ svc.Service) error {
	p.mu.Lock()
	p.stopping = true
	cancel := p.cancel
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	timer := time.NewTimer(p.stopTimeout)
	defer timer.Stop()
	select {
	case <-p.done:
		p.mu.Lock()
		err := p.err
		p.mu.Unlock()
		return err
	case <-timer.C:
		return fmt.Errorf("gateway did not stop within %s", p.stopTimeout)
	}
}

// programExitAction is what watch does after the gateway exits unexpectedly.
type programExitAction int

const (
	exitActionNone programExitAction = iota
	exitActionStopService
	exitActionExitProcess
)

// unexpectedExitAction decides how to terminate after an unexpected gateway
// exit. A foreground process has no unit to stop: Service.Stop may target an
// installed unit (and can succeed when it is already inactive) while this
// process stays blocked in Service.Run. Exit instead. Managed services ask the
// service manager to stop the process so it can restart it.
func unexpectedExitAction(interactive, hasService bool) programExitAction {
	if !hasService {
		return exitActionNone
	}
	if interactive {
		return exitActionExitProcess
	}
	return exitActionStopService
}

// watch reacts to a gateway that exits without a Stop request. Stop marks
// stopping before cancelling, so a normal shutdown never reaches this path.
func (p *Program) watch(s svc.Service) {
	<-p.done
	p.mu.Lock()
	err := p.err
	stopping := p.stopping
	p.mu.Unlock()
	if stopping {
		return
	}
	if err != nil {
		logger.Errorf("gateway exited unexpectedly: %v", err)
	} else {
		logger.Error("gateway exited unexpectedly")
	}

	switch unexpectedExitAction(p.interactive(), s != nil) {
	case exitActionNone:
		return
	case exitActionExitProcess:
		logger.Error("gateway exited unexpectedly; exiting foreground process")
		os.Exit(1)
	case exitActionStopService:
		if stopErr := s.Stop(); stopErr != nil {
			logger.Errorf("service stop after gateway exit failed: %v", stopErr)
			os.Exit(1)
		}
	}
}

func (p *Program) exitError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	return errors.New("gateway exited before reporting readiness")
}

// Run hosts program through the platform service manager. A foreground Windows
// process cannot rely on the SCM, so it uses signal handling directly.
func Run(spec Spec, program *Program) error {
	cfg, err := spec.ServiceConfig("")
	if err != nil {
		return err
	}
	s, err := svc.New(program, cfg)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" && svc.Interactive() {
		return runWindowsForeground(program, s)
	}
	return s.Run()
}

func runWindowsForeground(program *Program, s svc.Service) error {
	if err := program.Start(s); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
	return program.Stop(s)
}

// applyTimezone honours an explicit TZ value, normally loaded from .env before
// the gateway starts. Invalid zones warn and leave time.Local untouched.
func applyTimezone() {
	tz := os.Getenv("TZ")
	if tz == "" {
		return
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		logger.Warnf("invalid TZ %q: %v", tz, err)
		return
	}
	time.Local = loc //nolint:gosmopolitan // TZ is an explicit runtime override
}
