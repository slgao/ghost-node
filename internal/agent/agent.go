package agent

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/vpnplatform/core/pkg/config"
	"github.com/vpnplatform/core/pkg/logger"
	pb "github.com/vpnplatform/core/internal/grpc/proto"
)

// Agent is the main node-agent orchestrator.
type Agent struct {
	cfg            *config.AgentConfig
	nodeID         string
	conn           *grpc.ClientConn
	client         pb.NodeAgentServiceClient
	processManager *ProcessManager
	healthMonitor  *HealthMonitor
	configManager  *ConfigManager
}

func New(cfg *config.AgentConfig) *Agent {
	pm := NewProcessManager()
	return &Agent{
		cfg:            cfg,
		processManager: pm,
		healthMonitor:  NewHealthMonitor(pm),
		configManager:  NewConfigManager(pm),
	}
}

// Start connects to the control plane, registers the node, and enters
// the main supervision loop.
func (a *Agent) Start(ctx context.Context) error {
	if err := a.connect(ctx); err != nil {
		return fmt.Errorf("connecting to control plane: %w", err)
	}

	if err := a.register(ctx); err != nil {
		return fmt.Errorf("registering node: %w", err)
	}

	if err := a.fetchAndApplyConfig(ctx); err != nil {
		logger.L().Warn("initial config fetch failed", zap.Error(err))
	}

	go a.runHeartbeatLoop(ctx)
	go a.runConfigPollLoop(ctx)
	go a.healthMonitor.Run(ctx)

	logger.L().Info("node agent running", zap.String("node_id", a.nodeID))
	return nil
}

func (a *Agent) Stop(ctx context.Context) {
	a.processManager.StopAll(ctx)
	if a.conn != nil {
		_ = a.conn.Close()
	}
}

func (a *Agent) connect(ctx context.Context) error {
	// TODO: replace insecure with mTLS credentials once certs are provisioned
	conn, err := grpc.DialContext(ctx, a.cfg.ControlPlaneAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", a.cfg.ControlPlaneAddr, err)
	}
	a.conn = conn
	a.client = pb.NewNodeAgentServiceClient(conn)
	return nil
}

func (a *Agent) register(ctx context.Context) error {
	if a.cfg.NodeID != "" {
		a.nodeID = a.cfg.NodeID
		logger.L().Info("using existing node ID", zap.String("node_id", a.nodeID))
		return nil
	}

	hostname, _ := getHostname()
	resp, err := a.client.RegisterNode(ctx, &pb.RegisterNodeRequest{
		Hostname:     hostname,
		Address:      getOutboundIP(),
		Region:       "auto",
		AgentVersion: "0.1.0",
		SupportedTransports: []string{"xray", "hysteria2"},
	})
	if err != nil {
		return fmt.Errorf("RegisterNode RPC: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("registration failed: %s", resp.Message)
	}
	a.nodeID = resp.NodeId
	logger.L().Info("registered with control plane", zap.String("node_id", a.nodeID))
	return nil
}

func (a *Agent) runHeartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.sendHeartbeat(ctx); err != nil {
				logger.L().Warn("heartbeat failed", zap.Error(err))
			}
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) error {
	metrics := a.healthMonitor.CollectMetrics()
	resp, err := a.client.Heartbeat(ctx, &pb.HeartbeatRequest{
		NodeId:       a.nodeID,
		CpuUsage:     metrics.CPUUsage,
		MemUsage:     metrics.MemUsage,
		BandwidthIn:  metrics.BandwidthIn,
		BandwidthOut: metrics.BandwidthOut,
		ActiveConns:  int32(metrics.ActiveConns),
		AgentVersion: "0.1.0",
	})
	if err != nil {
		return err
	}
	if resp.ConfigChanged {
		logger.L().Info("config change detected, fetching new config")
		if err := a.fetchAndApplyConfig(ctx); err != nil {
			logger.L().Error("config reload failed", zap.Error(err))
		}
	}
	return nil
}

func (a *Agent) runConfigPollLoop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.ConfigPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.fetchAndApplyConfig(ctx); err != nil {
				logger.L().Warn("config poll failed", zap.Error(err))
			}
		}
	}
}

func (a *Agent) fetchAndApplyConfig(ctx context.Context) error {
	resp, err := a.client.GetConfig(ctx, &pb.GetConfigRequest{NodeId: a.nodeID})
	if err != nil {
		return fmt.Errorf("GetConfig RPC: %w", err)
	}
	return a.configManager.Apply(ctx, resp.Configs)
}
