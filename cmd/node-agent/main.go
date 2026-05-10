package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vpnplatform/core/internal/agent"
	"github.com/vpnplatform/core/pkg/config"
	"github.com/vpnplatform/core/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAgent("node-agent")
	if err != nil {
		return fmt.Errorf("loading agent config: %w", err)
	}

	logger.Init("info")
	defer logger.Sync()

	log := logger.Named("agent-main")
	log.Info("starting node-agent", zap.String("control_plane", cfg.ControlPlaneAddr))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := agent.New(cfg)

	if err := a.Start(ctx); err != nil {
		return fmt.Errorf("agent start: %w", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("node-agent shutting down...")
	a.Stop(ctx)
	return nil
}
