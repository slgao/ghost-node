package transport

import (
	"context"
	"time"
)

// Status represents the current state of a transport provider.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusError    Status = "error"
)

// Provider is the interface every transport engine must implement.
type Provider interface {
	// Name returns the transport identifier (e.g. "xray", "hysteria2", "wireguard").
	Name() string

	// Start launches the transport process and blocks until it is ready or fails.
	Start(ctx context.Context) error

	// Stop gracefully terminates the transport process.
	Stop(ctx context.Context) error

	// HealthCheck probes the transport and returns a non-nil error if unhealthy.
	HealthCheck(ctx context.Context) error

	// GetLatency measures round-trip latency through the transport.
	GetLatency(ctx context.Context) (time.Duration, error)

	// ReloadConfig hot-reloads configuration without a full restart where possible.
	ReloadConfig(ctx context.Context) error

	// GetStatus returns the current operational status.
	GetStatus() Status
}

// HealthResult captures the outcome of a single health-check cycle.
type HealthResult struct {
	Transport string
	Status    Status
	Latency   time.Duration
	Error     error
	CheckedAt time.Time
}
