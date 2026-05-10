package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ─── Metric declarations ──────────────────────────────────────────────────────

var (
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "vpnplatform",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request latency by method, path, and status code.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "vpnplatform",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	AuthEvents = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "vpnplatform",
			Subsystem: "auth",
			Name:      "events_total",
			Help:      "Authentication events (register, login, logout, refresh).",
		},
		[]string{"event", "success"},
	)

	ActiveSessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "vpnplatform",
			Subsystem: "vpn",
			Name:      "active_sessions",
			Help:      "Number of currently active VPN sessions.",
		},
	)

	NodeStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "vpnplatform",
			Subsystem: "node",
			Name:      "status",
			Help:      "Node online status (1 = online, 0 = offline) by region.",
		},
		[]string{"node_id", "region", "transport"},
	)

	NodeLatencyMs = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "vpnplatform",
			Subsystem: "node",
			Name:      "latency_ms",
			Help:      "Last measured latency to node in milliseconds.",
		},
		[]string{"node_id", "region"},
	)

	BandwidthBytesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "vpnplatform",
			Subsystem: "traffic",
			Name:      "bytes_total",
			Help:      "Total VPN traffic bytes.",
		},
		[]string{"direction"}, // "in" | "out"
	)

	RegisteredUsers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "vpnplatform",
			Subsystem: "users",
			Name:      "registered_total",
			Help:      "Total number of registered users.",
		},
	)

	RegisteredNodes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "vpnplatform",
			Subsystem: "nodes",
			Name:      "registered_total",
			Help:      "Total number of registered nodes.",
		},
	)
)

// ─── Gin middleware ───────────────────────────────────────────────────────────

// GinMiddleware records HTTP request duration and total count for every request.
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		duration := time.Since(start).Seconds()

		HTTPRequestDuration.WithLabelValues(c.Request.Method, path, status).Observe(duration)
		HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
	}
}

// ─── Helpers called by service layer ──────────────────────────────────────────

func RecordAuthEvent(event string, success bool) {
	s := "true"
	if !success {
		s = "false"
	}
	AuthEvents.WithLabelValues(event, s).Inc()
}

func RecordSessionStart() { ActiveSessions.Inc() }
func RecordSessionEnd()   { ActiveSessions.Dec() }

func RecordTraffic(in, out int64) {
	if in > 0 {
		BandwidthBytesTotal.WithLabelValues("in").Add(float64(in))
	}
	if out > 0 {
		BandwidthBytesTotal.WithLabelValues("out").Add(float64(out))
	}
}
