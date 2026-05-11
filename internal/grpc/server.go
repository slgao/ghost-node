package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/google/uuid"
	"github.com/vpnplatform/core/internal/models"
	"github.com/vpnplatform/core/internal/repository"
	"github.com/vpnplatform/core/pkg/config"
	"github.com/vpnplatform/core/pkg/logger"

	pb "github.com/vpnplatform/core/internal/grpc/proto"
)

// AgentServer implements pb.NodeAgentServiceServer.
type AgentServer struct {
	pb.UnimplementedNodeAgentServiceServer
	nodeRepo repository.NodeRepository
}

func NewAgentServer(nodeRepo repository.NodeRepository) *AgentServer {
	return &AgentServer{nodeRepo: nodeRepo}
}

func (s *AgentServer) RegisterNode(ctx context.Context, req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
	// Upsert: reuse existing record if address already registered.
	existing, err := s.nodeRepo.FindByAddress(ctx, req.Address)
	if err == nil {
		existing.AgentVersion = req.AgentVersion
		existing.Status = models.NodeStatusOnline
		if req.Hostname != "" {
			existing.Name = req.Hostname
		}
		if updateErr := s.nodeRepo.Update(ctx, existing); updateErr != nil {
			logger.L().Warn("node re-register update failed", zap.Error(updateErr))
		}
		logger.L().Info("node re-registered", zap.String("id", existing.ID.String()), zap.String("host", req.Hostname))
		return &pb.RegisterNodeResponse{NodeId: existing.ID.String(), Success: true, Message: "reconnected"}, nil
	}

	node := &models.Node{
		Name:         req.Hostname,
		Address:      req.Address,
		Region:       req.Region,
		Country:      req.Country,
		AgentVersion: req.AgentVersion,
		Status:       models.NodeStatusOnline,
	}
	if err := s.nodeRepo.Create(ctx, node); err != nil {
		logger.L().Error("failed to register node", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "registering node: %v", err)
	}

	logger.L().Info("node registered", zap.String("id", node.ID.String()), zap.String("host", req.Hostname))
	return &pb.RegisterNodeResponse{NodeId: node.ID.String(), Success: true, Message: "registered"}, nil
}

// StartStaleDetection runs a background loop that marks nodes offline when
// their heartbeat has not been received within threshold.
func (s *AgentServer) StartStaleDetection(ctx context.Context, interval, threshold time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := s.nodeRepo.MarkStaleOffline(ctx, threshold)
				if err != nil {
					logger.L().Warn("stale node detection failed", zap.Error(err))
				} else if n > 0 {
					logger.L().Info("marked stale nodes offline", zap.Int64("count", n))
				}
			}
		}
	}()
}

func (s *AgentServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	id, err := uuid.Parse(req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid node id: %v", err)
	}

	metrics := models.Node{
		CPUUsage:     req.CpuUsage,
		MemUsage:     req.MemUsage,
		BandwidthIn:  req.BandwidthIn,
		BandwidthOut: req.BandwidthOut,
		ActiveConns:  int(req.ActiveConns),
		AgentVersion: req.AgentVersion,
	}

	if err := s.nodeRepo.UpdateHeartbeat(ctx, id, metrics); err != nil {
		logger.L().Warn("heartbeat update failed", zap.String("node", req.NodeId), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "updating heartbeat: %v", err)
	}

	return &pb.HeartbeatResponse{ConfigChanged: false}, nil
}

func (s *AgentServer) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	id, err := uuid.Parse(req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid node id: %v", err)
	}

	profiles, err := s.nodeRepo.ListTransportProfiles(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fetching profiles: %v", err)
	}

	configs := make([]*pb.TransportConfig, 0, len(profiles))
	for _, p := range profiles {
		cfgJSON, _ := json.Marshal(p.Config)
		configs = append(configs, &pb.TransportConfig{
			TransportType: string(p.Type),
			Port:          int32(p.Port),
			ConfigJson:    cfgJSON,
			Enabled:       p.IsActive,
		})
	}

	return &pb.GetConfigResponse{Configs: configs}, nil
}

func (s *AgentServer) ReportMetrics(stream pb.NodeAgentService_ReportMetricsServer) error {
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.MetricsAck{Received: true})
		}
		if err != nil {
			return err
		}
		logger.L().Debug("metrics received",
			zap.String("node", event.NodeId),
			zap.Int64("bytes_in", event.BytesIn),
			zap.Int64("bytes_out", event.BytesOut),
		)
	}
}

// ─── Server lifecycle ─────────────────────────────────────────────────────

type Server struct {
	grpcServer *grpc.Server
	cfg        config.GRPCConfig
}

func NewServer(cfg config.GRPCConfig, agentServer *AgentServer) *Server {
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(4 * 1024 * 1024),
	}
	srv := grpc.NewServer(opts...)
	pb.RegisterNodeAgentServiceServer(srv, agentServer)
	return &Server{grpcServer: srv, cfg: cfg}
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gRPC listen on %s: %w", addr, err)
	}

	logger.L().Info("gRPC server listening", zap.String("addr", addr))

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			logger.L().Error("gRPC serve error", zap.Error(err))
		}
	}()
	return nil
}

func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ctx
	s.grpcServer.GracefulStop()
}
