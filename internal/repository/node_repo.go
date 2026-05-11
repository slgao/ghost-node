package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vpnplatform/core/internal/models"
)

type NodeRepository interface {
	Create(ctx context.Context, node *models.Node) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Node, error)
	FindByAddress(ctx context.Context, address string) (*models.Node, error)
	Update(ctx context.Context, node *models.Node) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListOnline(ctx context.Context) ([]models.Node, error)
	ListAll(ctx context.Context) ([]models.Node, error)
	UpdateHeartbeat(ctx context.Context, id uuid.UUID, metrics models.Node) error
	MarkStaleOffline(ctx context.Context, threshold time.Duration) (int64, error)
	CreateTransportProfile(ctx context.Context, tp *models.TransportProfile) error
	ListTransportProfiles(ctx context.Context, nodeID uuid.UUID) ([]models.TransportProfile, error)
}

type nodeRepository struct {
	db *gorm.DB
}

func NewNodeRepository(db *gorm.DB) NodeRepository {
	return &nodeRepository{db: db}
}

func (r *nodeRepository) Create(ctx context.Context, node *models.Node) error {
	if err := r.db.WithContext(ctx).Create(node).Error; err != nil {
		return fmt.Errorf("creating node: %w", err)
	}
	return nil
}

func (r *nodeRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Node, error) {
	var node models.Node
	err := r.db.WithContext(ctx).
		Preload("TransportProfiles").
		First(&node, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finding node: %w", err)
	}
	return &node, nil
}

func (r *nodeRepository) Update(ctx context.Context, node *models.Node) error {
	if err := r.db.WithContext(ctx).Save(node).Error; err != nil {
		return fmt.Errorf("updating node: %w", err)
	}
	return nil
}

func (r *nodeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&models.Node{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("deleting node: %w", err)
	}
	return nil
}

func (r *nodeRepository) ListOnline(ctx context.Context) ([]models.Node, error) {
	var nodes []models.Node
	err := r.db.WithContext(ctx).
		Preload("TransportProfiles", "is_active = ?", true).
		Where("status = ? AND is_public = ?", models.NodeStatusOnline, true).
		Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("listing online nodes: %w", err)
	}
	return nodes, nil
}

func (r *nodeRepository) ListAll(ctx context.Context) ([]models.Node, error) {
	var nodes []models.Node
	if err := r.db.WithContext(ctx).Preload("TransportProfiles").Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	return nodes, nil
}

func (r *nodeRepository) UpdateHeartbeat(ctx context.Context, id uuid.UUID, m models.Node) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":          models.NodeStatusOnline,
		"last_heartbeat":  now,
		"cpu_usage":       m.CPUUsage,
		"mem_usage":       m.MemUsage,
		"bandwidth_in":    m.BandwidthIn,
		"bandwidth_out":   m.BandwidthOut,
		"active_conns":    m.ActiveConns,
		"agent_version":   m.AgentVersion,
	}
	err := r.db.WithContext(ctx).
		Model(&models.Node{}).
		Where("id = ?", id).
		Updates(updates).Error
	if err != nil {
		return fmt.Errorf("updating heartbeat: %w", err)
	}
	return nil
}

func (r *nodeRepository) CreateTransportProfile(ctx context.Context, tp *models.TransportProfile) error {
	if err := r.db.WithContext(ctx).Create(tp).Error; err != nil {
		return fmt.Errorf("creating transport profile: %w", err)
	}
	return nil
}

func (r *nodeRepository) FindByAddress(ctx context.Context, address string) (*models.Node, error) {
	var node models.Node
	err := r.db.WithContext(ctx).
		Where("address = ?", address).
		First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finding node by address: %w", err)
	}
	return &node, nil
}

func (r *nodeRepository) MarkStaleOffline(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().Add(-threshold)
	// Only affect nodes that have had an agent heartbeat before — manually-added
	// nodes (agent_version IS NULL) are never auto-offlined.
	result := r.db.WithContext(ctx).
		Model(&models.Node{}).
		Where("status = ? AND last_heartbeat < ? AND agent_version IS NOT NULL", models.NodeStatusOnline, cutoff).
		Update("status", models.NodeStatusOffline)
	if result.Error != nil {
		return 0, fmt.Errorf("marking stale nodes offline: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *nodeRepository) ListTransportProfiles(ctx context.Context, nodeID uuid.UUID) ([]models.TransportProfile, error) {
	var profiles []models.TransportProfile
	err := r.db.WithContext(ctx).
		Where("node_id = ? AND is_active = ?", nodeID, true).
		Order("priority ASC").
		Find(&profiles).Error
	if err != nil {
		return nil, fmt.Errorf("listing transport profiles: %w", err)
	}
	return profiles, nil
}
