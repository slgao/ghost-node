package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"go.uber.org/zap"

	"github.com/vpnplatform/core/pkg/logger"
)

// Metrics is a snapshot of node resource usage.
type Metrics struct {
	CPUUsage     float64
	MemUsage     float64
	BandwidthIn  int64
	BandwidthOut int64
	ActiveConns  int
	CollectedAt  time.Time
}

// HealthMonitor periodically collects node and transport metrics.
type HealthMonitor struct {
	pm      *ProcessManager
	latest  Metrics
}

func NewHealthMonitor(pm *ProcessManager) *HealthMonitor {
	return &HealthMonitor{pm: pm}
}

// Run polls metrics every 15 s.
func (h *HealthMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.collect()
		}
	}
}

func (h *HealthMonitor) collect() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	totalMem := float64(memStats.Sys)
	usedMem := float64(memStats.Alloc)
	memPct := 0.0
	if totalMem > 0 {
		memPct = (usedMem / totalMem) * 100
	}

	h.latest = Metrics{
		CPUUsage:    h.readCPU(),
		MemUsage:    memPct,
		CollectedAt: time.Now(),
	}
	logger.L().Debug("metrics collected",
		zap.Float64("cpu", h.latest.CPUUsage),
		zap.Float64("mem", h.latest.MemUsage),
	)
}

// CollectMetrics returns the most recently collected metrics snapshot.
func (h *HealthMonitor) CollectMetrics() Metrics {
	if h.latest.CollectedAt.IsZero() {
		h.collect()
	}
	return h.latest
}

// readCPU reads /proc/stat on Linux; returns 0 on other platforms.
func (h *HealthMonitor) readCPU() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	var user, nice, system, idle, iowait, irq, softirq uint64
	_, err = fmt.Sscanf(string(data), "cpu %d %d %d %d %d %d %d",
		&user, &nice, &system, &idle, &iowait, &irq, &softirq)
	if err != nil {
		return 0
	}

	total := user + nice + system + idle + iowait + irq + softirq
	busy := total - idle - iowait
	if total == 0 {
		return 0
	}
	return float64(busy) / float64(total) * 100
}
