package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	pb "github.com/vpnplatform/core/internal/grpc/proto"
	"github.com/vpnplatform/core/pkg/logger"
)

const (
	xrayConfigDir      = "/etc/xray"
	hysteria2ConfigDir = "/etc/hysteria"
)

// ConfigManager applies transport configurations received from the control plane.
type ConfigManager struct {
	pm *ProcessManager
}

func NewConfigManager(pm *ProcessManager) *ConfigManager {
	return &ConfigManager{pm: pm}
}

// Apply writes config files to disk and (re)starts the relevant processes.
func (cm *ConfigManager) Apply(ctx context.Context, configs []*pb.TransportConfig) error {
	for _, cfg := range configs {
		if err := cm.applyOne(ctx, cfg); err != nil {
			logger.L().Error("failed to apply transport config",
				zap.String("transport", cfg.TransportType), zap.Error(err))
		}
	}
	return nil
}

func (cm *ConfigManager) applyOne(ctx context.Context, cfg *pb.TransportConfig) error {
	if !cfg.Enabled {
		return cm.pm.Stop(ctx, cfg.TransportType)
	}

	switch cfg.TransportType {
	case "xray":
		return cm.applyXray(ctx, cfg)
	case "hysteria2":
		return cm.applyHysteria2(ctx, cfg)
	default:
		logger.L().Warn("unknown transport type, skipping", zap.String("transport", cfg.TransportType))
		return nil
	}
}

func (cm *ConfigManager) applyXray(ctx context.Context, cfg *pb.TransportConfig) error {
	configPath := filepath.Join(xrayConfigDir, "config.json")
	if err := os.MkdirAll(xrayConfigDir, 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", xrayConfigDir, err)
	}
	if err := os.WriteFile(configPath, cfg.ConfigJson, 0600); err != nil {
		return fmt.Errorf("writing xray config: %w", err)
	}

	if cm.pm.IsRunning("xray") {
		// hot-reload: stop then start (xray doesn't support SIGHUP in all versions)
		if err := cm.pm.Stop(ctx, "xray"); err != nil {
			return err
		}
	}
	return cm.pm.Start(ctx, "xray", "/usr/local/bin/xray", []string{"run", "-c", configPath})
}

func (cm *ConfigManager) applyHysteria2(ctx context.Context, cfg *pb.TransportConfig) error {
	configPath := filepath.Join(hysteria2ConfigDir, "config.yaml")
	if err := os.MkdirAll(hysteria2ConfigDir, 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", hysteria2ConfigDir, err)
	}
	if err := os.WriteFile(configPath, cfg.ConfigJson, 0600); err != nil {
		return fmt.Errorf("writing hysteria2 config: %w", err)
	}

	if cm.pm.IsRunning("hysteria2") {
		if err := cm.pm.Stop(ctx, "hysteria2"); err != nil {
			return err
		}
	}
	return cm.pm.Start(ctx, "hysteria2", "/usr/local/bin/hysteria", []string{"server", "-c", configPath})
}
