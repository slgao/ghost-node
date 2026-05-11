package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/vpnplatform/core/internal/auth"
	"github.com/vpnplatform/core/internal/database"
	grpcserver "github.com/vpnplatform/core/internal/grpc"
	"github.com/vpnplatform/core/internal/handler"
	"github.com/vpnplatform/core/internal/metrics"
	"github.com/vpnplatform/core/internal/repository"
	"github.com/vpnplatform/core/internal/service"
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
	cfg, err := config.Load("control-plane")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger.Init(cfg.Observability.LogLevel)
	defer logger.Sync()

	log := logger.Named("main")
	log.Info("starting control-plane", zap.Int("port", cfg.Server.Port))

	// ── Background context (cancelled on shutdown) ────────────────────────────
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	// ── Database ──────────────────────────────────────────────────────────────
	db, err := database.NewPostgres(cfg.Database)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}

	if err := database.Migrate(db); err != nil {
		return fmt.Errorf("migration: %w", err)
	}

	rdb, err := database.NewRedis(cfg.Redis)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	// ── Repositories ──────────────────────────────────────────────────────────
	userRepo    := repository.NewUserRepository(db)
	nodeRepo    := repository.NewNodeRepository(db)
	deviceRepo  := repository.NewDeviceRepository(db)
	trafficRepo := repository.NewTrafficRepository(db)

	// ── Services ──────────────────────────────────────────────────────────────
	jwtSvc     := auth.NewJWTService(cfg.JWT)
	authSvc    := service.NewAuthService(userRepo, jwtSvc, db)
	nodeSvc    := service.NewNodeService(nodeRepo)
	userSvc    := service.NewUserService(userRepo, deviceRepo)
	trafficSvc := service.NewTrafficService(trafficRepo, db)

	// Seed gauges from DB so metrics survive process restarts.
	if n, err := userRepo.Count(bgCtx); err == nil {
		metrics.RegisteredUsers.Set(float64(n))
	}

	// ── HTTP server ───────────────────────────────────────────────────────────
	adminH  := handler.NewAdminHandler(userRepo, db)
	router  := handler.NewRouter(jwtSvc, authSvc, nodeSvc, userSvc, trafficSvc, adminH, rdb)
	engine := router.Setup()

	httpAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpSrv := &http.Server{
		Addr:         httpAddr,
		Handler:      engine,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// ── gRPC server ───────────────────────────────────────────────────────────
	agentServer := grpcserver.NewAgentServer(nodeRepo)
	grpcSrv := grpcserver.NewServer(cfg.GRPC, agentServer)

	if err := grpcSrv.Start(); err != nil {
		return fmt.Errorf("gRPC server start: %w", err)
	}

	// Mark nodes offline after 2 missed heartbeats (default interval 30s → threshold 2min).
	agentServer.StartStaleDetection(bgCtx, 30*time.Second, 2*time.Minute)

	// ── Start HTTP ────────────────────────────────────────────────────────────
	go func() {
		log.Info("HTTP listening", zap.String("addr", httpAddr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down...")
	bgCancel()
	grpcSrv.Stop()

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return httpSrv.Shutdown(shutCtx)
}
