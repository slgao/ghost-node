package transport

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vpnplatform/core/pkg/logger"
)

// Manager supervises multiple transport providers and handles automatic
// failover and health monitoring.
type Manager struct {
	mu        sync.RWMutex
	providers map[string]Provider
	active    string
	stopCh    chan struct{}
}

func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]Provider),
		stopCh:    make(chan struct{}),
	}
}

// Register adds a provider. Must be called before Start.
func (m *Manager) Register(p Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[p.Name()] = p
}

// StartAll launches all registered providers.
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, p := range m.providers {
		if err := p.Start(ctx); err != nil {
			logger.L().Warn("transport start failed", zap.String("transport", name), zap.Error(err))
			continue
		}
		if m.active == "" {
			m.active = name
		}
		logger.L().Info("transport started", zap.String("transport", name))
	}
	return nil
}

// StopAll gracefully stops all providers.
func (m *Manager) StopAll(ctx context.Context) {
	close(m.stopCh)
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, p := range m.providers {
		if err := p.Stop(ctx); err != nil {
			logger.L().Warn("transport stop error", zap.String("transport", name), zap.Error(err))
		}
	}
}

// RunHealthLoop polls all providers every interval and switches active transport
// if the current one becomes unhealthy.
func (m *Manager) RunHealthLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAndFailover(ctx)
		}
	}
}

func (m *Manager) checkAndFailover(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	results := make([]HealthResult, 0, len(m.providers))
	for name, p := range m.providers {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := p.HealthCheck(checkCtx)
		cancel()

		lat := time.Duration(0)
		if err == nil {
			latCtx, cancel2 := context.WithTimeout(ctx, 5*time.Second)
			lat, _ = p.GetLatency(latCtx)
			cancel2()
		}

		results = append(results, HealthResult{
			Transport: name,
			Status:    p.GetStatus(),
			Latency:   lat,
			Error:     err,
			CheckedAt: time.Now(),
		})

		if err != nil {
			logger.L().Warn("transport unhealthy", zap.String("transport", name), zap.Error(err))
		}
	}

	// if current active is unhealthy, pick the lowest-latency healthy one
	if activeErr := m.activeHealthy(results); activeErr != nil {
		m.selectBest(results)
	}
}

func (m *Manager) activeHealthy(results []HealthResult) error {
	for _, r := range results {
		if r.Transport == m.active {
			return r.Error
		}
	}
	return nil
}

func (m *Manager) selectBest(results []HealthResult) {
	var best *HealthResult
	for i := range results {
		r := &results[i]
		if r.Error != nil {
			continue
		}
		if best == nil || r.Latency < best.Latency {
			best = r
		}
	}
	if best != nil && best.Transport != m.active {
		logger.L().Info("switching active transport",
			zap.String("from", m.active),
			zap.String("to", best.Transport),
			zap.Duration("latency", best.Latency),
		)
		m.active = best.Transport
	}
}

// ActiveTransport returns the name of the currently active transport.
func (m *Manager) ActiveTransport() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// GetProvider returns a registered provider by name.
func (m *Manager) GetProvider(name string) (Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[name]
	return p, ok
}

// HealthResults returns the latest health state of all providers.
func (m *Manager) HealthResults(ctx context.Context) []HealthResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]HealthResult, 0, len(m.providers))
	for name, p := range m.providers {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := p.HealthCheck(checkCtx)
		cancel()

		lat := time.Duration(0)
		if err == nil {
			latCtx, cancel2 := context.WithTimeout(ctx, 5*time.Second)
			lat, _ = p.GetLatency(latCtx)
			cancel2()
		}

		results = append(results, HealthResult{
			Transport: name,
			Status:    p.GetStatus(),
			Latency:   lat,
			Error:     err,
			CheckedAt: time.Now(),
		})
	}
	return results
}
