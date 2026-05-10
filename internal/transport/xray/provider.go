package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vpnplatform/core/internal/transport"
	"github.com/vpnplatform/core/pkg/logger"
)

// Provider manages a single xray-core process.
type Provider struct {
	mu         sync.Mutex
	binPath    string
	configPath string
	apiPort    int    // xray API port for health checks
	cmd        *exec.Cmd
	status     transport.Status
}

func New(binPath, configPath string, apiPort int) *Provider {
	return &Provider{
		binPath:    binPath,
		configPath: configPath,
		apiPort:    apiPort,
		status:     transport.StatusStopped,
	}
}

func (p *Provider) Name() string { return "xray" }

func (p *Provider) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status == transport.StatusRunning {
		return nil
	}

	if _, err := os.Stat(p.binPath); err != nil {
		return fmt.Errorf("xray binary not found at %s: %w", p.binPath, err)
	}

	p.status = transport.StatusStarting

	p.cmd = exec.CommandContext(ctx, p.binPath, "run", "-c", p.configPath)
	p.cmd.Stdout = newPrefixedWriter("xray", os.Stdout)
	p.cmd.Stderr = newPrefixedWriter("xray", os.Stderr)

	if err := p.cmd.Start(); err != nil {
		p.status = transport.StatusError
		return fmt.Errorf("starting xray process: %w", err)
	}

	// wait up to 5s for xray API to become available
	if err := p.waitReady(5 * time.Second); err != nil {
		_ = p.cmd.Process.Kill()
		p.status = transport.StatusError
		return fmt.Errorf("xray did not become ready: %w", err)
	}

	p.status = transport.StatusRunning
	logger.L().Info("xray started", zap.Int("pid", p.cmd.Process.Pid))

	// reap the process asynchronously
	go func() {
		_ = p.cmd.Wait()
		p.mu.Lock()
		p.status = transport.StatusStopped
		p.mu.Unlock()
		logger.L().Warn("xray process exited")
	}()

	return nil
}

func (p *Provider) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil || p.cmd.Process == nil {
		p.status = transport.StatusStopped
		return nil
	}

	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		_ = p.cmd.Process.Kill()
	}

	done := make(chan struct{})
	go func() {
		_ = p.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
	case <-ctx.Done():
		_ = p.cmd.Process.Kill()
	}

	p.status = transport.StatusStopped
	return nil
}

func (p *Provider) HealthCheck(ctx context.Context) error {
	p.mu.Lock()
	s := p.status
	p.mu.Unlock()

	if s != transport.StatusRunning {
		return fmt.Errorf("xray not running (status: %s)", s)
	}

	// verify xray's API port is accepting TCP connections
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", p.apiPort))
	if err != nil {
		return fmt.Errorf("xray API port unreachable: %w", err)
	}
	conn.Close()
	return nil
}

func (p *Provider) GetLatency(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	if err := p.HealthCheck(ctx); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (p *Provider) ReloadConfig(ctx context.Context) error {
	// xray supports hot-reload via a SIGHUP or API call; we restart for simplicity at MVP
	if err := p.Stop(ctx); err != nil {
		return err
	}
	return p.Start(ctx)
}

func (p *Provider) GetStatus() transport.Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// GenerateConfig writes a minimal xray VLESS+TCP config to disk.
func GenerateConfig(configDir string, port int, userIDs []string) error {
	clients := make([]map[string]interface{}, 0, len(userIDs))
	for _, id := range userIDs {
		clients = append(clients, map[string]interface{}{"id": id, "level": 0})
	}

	cfg := map[string]interface{}{
		"log": map[string]interface{}{"loglevel": "warning"},
		"inbounds": []interface{}{
			map[string]interface{}{
				"port":     port,
				"protocol": "vless",
				"settings": map[string]interface{}{
					"clients":    clients,
					"decryption": "none",
				},
				"streamSettings": map[string]interface{}{
					"network": "tcp",
				},
			},
		},
		"outbounds": []interface{}{
			map[string]interface{}{"protocol": "freedom"},
		},
		"api": map[string]interface{}{
			"tag":      "api",
			"services": []string{"HandlerService", "StatsService"},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling xray config: %w", err)
	}

	path := filepath.Join(configDir, "config.json")
	return os.WriteFile(path, data, 0600)
}

func (p *Provider) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", p.apiPort))
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for xray API on port %d", p.apiPort)
}

// prefixedWriter prepends a tag to each line written to an underlying writer.
type prefixedWriter struct {
	tag string
	w   *os.File
}

func newPrefixedWriter(tag string, w *os.File) *prefixedWriter {
	return &prefixedWriter{tag: tag, w: w}
}

func (pw *prefixedWriter) Write(p []byte) (int, error) {
	return fmt.Fprintf(pw.w, "[%s] %s", pw.tag, p)
}
