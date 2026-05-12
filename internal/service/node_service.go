package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"

	"github.com/vpnplatform/core/internal/models"
	"github.com/vpnplatform/core/internal/repository"
)

type NodeService struct {
	nodeRepo repository.NodeRepository
}

func NewNodeService(nodeRepo repository.NodeRepository) *NodeService {
	return &NodeService{nodeRepo: nodeRepo}
}

type CreateNodeInput struct {
	Name    string
	Address string
	Region  string
	Country string
}

func (s *NodeService) Create(ctx context.Context, in CreateNodeInput) (*models.Node, error) {
	node := &models.Node{
		Name:    in.Name,
		Address: in.Address,
		Region:  in.Region,
		Country: in.Country,
		Status:  models.NodeStatusOffline,
	}
	if err := s.nodeRepo.Create(ctx, node); err != nil {
		return nil, fmt.Errorf("creating node: %w", err)
	}
	return node, nil
}

func (s *NodeService) GetByID(ctx context.Context, id uuid.UUID) (*models.Node, error) {
	node, err := s.nodeRepo.FindByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, errors.New("node not found")
	}
	return node, err
}

func (s *NodeService) ListOnline(ctx context.Context) ([]models.Node, error) {
	return s.nodeRepo.ListOnline(ctx)
}

func (s *NodeService) ListAll(ctx context.Context) ([]models.Node, error) {
	return s.nodeRepo.ListAll(ctx)
}

func (s *NodeService) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.nodeRepo.FindByID(ctx, id); err != nil {
		return errors.New("node not found")
	}
	return s.nodeRepo.Delete(ctx, id)
}

// AddTransportProfile attaches a transport configuration to a node.
func (s *NodeService) AddTransportProfile(ctx context.Context, nodeID uuid.UUID, tp *models.TransportProfile) error {
	if _, err := s.nodeRepo.FindByID(ctx, nodeID); err != nil {
		return errors.New("node not found")
	}
	tp.NodeID = nodeID
	return s.nodeRepo.CreateTransportProfile(ctx, tp)
}

// GetConnectionConfig returns the best available transport profile for the user to connect to.
func (s *NodeService) GetConnectionConfig(ctx context.Context, nodeID uuid.UUID) (*models.TransportProfile, error) {
	profiles, err := s.nodeRepo.ListTransportProfiles(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("fetching transport profiles: %w", err)
	}
	if len(profiles) == 0 {
		return nil, errors.New("no active transport profiles for this node")
	}
	// profiles are ordered by priority ASC, so first is best
	return &profiles[0], nil
}

// SelectBestNode picks the least-loaded online node and its top transport profile.
// Score: CPU 50% + memory 30% + connections 20% (all normalised to 0–1). Lower = better.
func (s *NodeService) SelectBestNode(ctx context.Context) (*models.Node, *models.TransportProfile, error) {
	nodes, err := s.nodeRepo.ListOnline(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing online nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil, nil, errors.New("no online nodes available")
	}

	type candidate struct {
		node  models.Node
		score float64
	}
	var pool []candidate
	for _, n := range nodes {
		if len(n.TransportProfiles) == 0 {
			continue
		}
		score := (n.CPUUsage/100)*0.5 +
			(n.MemUsage/100)*0.3 +
			(math.Min(float64(n.ActiveConns), 1000)/1000)*0.2
		pool = append(pool, candidate{node: n, score: score})
	}
	if len(pool) == 0 {
		return nil, nil, errors.New("no online nodes with transport profiles")
	}

	sort.Slice(pool, func(i, j int) bool { return pool[i].score < pool[j].score })
	best := pool[0].node

	profiles, err := s.nodeRepo.ListTransportProfiles(ctx, best.ID)
	if err != nil || len(profiles) == 0 {
		return nil, nil, errors.New("no transport profiles for selected node")
	}
	return &best, &profiles[0], nil
}
