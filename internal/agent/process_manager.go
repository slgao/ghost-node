package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"go.uber.org/zap"

	"github.com/vpnplatform/core/pkg/logger"
)

type processEntry struct {
	name string
	bin  string
	args []string
	cmd  *exec.Cmd
}

// ProcessManager starts, monitors, and restarts transport processes.
type ProcessManager struct {
	mu        sync.Mutex
	processes map[string]*processEntry
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*processEntry),
	}
}

// Start launches a named process and tracks it.
func (pm *ProcessManager) Start(ctx context.Context, name, bin string, args []string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if entry, ok := pm.processes[name]; ok && entry.cmd != nil && entry.cmd.Process != nil {
		// already running
		return nil
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = newTaggedWriter(name, os.Stdout)
	cmd.Stderr = newTaggedWriter(name, os.Stderr)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", name, err)
	}

	entry := &processEntry{name: name, bin: bin, args: args, cmd: cmd}
	pm.processes[name] = entry

	// watch for unexpected exits and restart
	go pm.watch(ctx, entry)

	logger.L().Info("process started", zap.String("name", name), zap.Int("pid", cmd.Process.Pid))
	return nil
}

func (pm *ProcessManager) watch(ctx context.Context, e *processEntry) {
	err := e.cmd.Wait()
	if ctx.Err() != nil {
		return // context cancelled, not an unexpected exit
	}
	logger.L().Warn("process exited unexpectedly, restarting",
		zap.String("name", e.name), zap.Error(err))

	pm.mu.Lock()
	delete(pm.processes, e.name)
	pm.mu.Unlock()

	if startErr := pm.Start(ctx, e.name, e.bin, e.args); startErr != nil {
		logger.L().Error("restart failed", zap.String("name", e.name), zap.Error(startErr))
	}
}

// Stop sends SIGTERM then SIGKILL to a named process.
func (pm *ProcessManager) Stop(ctx context.Context, name string) error {
	pm.mu.Lock()
	entry, ok := pm.processes[name]
	pm.mu.Unlock()

	if !ok || entry.cmd == nil || entry.cmd.Process == nil {
		return nil
	}

	_ = entry.cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		_ = entry.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		_ = entry.cmd.Process.Kill()
	}

	pm.mu.Lock()
	delete(pm.processes, name)
	pm.mu.Unlock()
	return nil
}

// StopAll stops every tracked process.
func (pm *ProcessManager) StopAll(ctx context.Context) {
	pm.mu.Lock()
	names := make([]string, 0, len(pm.processes))
	for n := range pm.processes {
		names = append(names, n)
	}
	pm.mu.Unlock()

	for _, n := range names {
		_ = pm.Stop(ctx, n)
	}
}

// IsRunning reports whether the named process is currently running.
func (pm *ProcessManager) IsRunning(name string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	e, ok := pm.processes[name]
	return ok && e.cmd != nil && e.cmd.Process != nil
}

type taggedWriter struct {
	tag string
	w   *os.File
}

func newTaggedWriter(tag string, w *os.File) *taggedWriter { return &taggedWriter{tag: tag, w: w} }

func (tw *taggedWriter) Write(p []byte) (int, error) {
	return fmt.Fprintf(tw.w, "[%s] %s", tw.tag, p)
}
