package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vpnplatform/core/internal/models"
)

type DeviceRepository interface {
	Create(ctx context.Context, device *models.Device) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Device, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Device, error)
	CountByUser(ctx context.Context, userID uuid.UUID) (int64, error)
	Update(ctx context.Context, device *models.Device) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type deviceRepository struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) DeviceRepository {
	return &deviceRepository{db: db}
}

func (r *deviceRepository) Create(ctx context.Context, device *models.Device) error {
	if err := r.db.WithContext(ctx).Create(device).Error; err != nil {
		return fmt.Errorf("creating device: %w", err)
	}
	return nil
}

func (r *deviceRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Device, error) {
	var device models.Device
	err := r.db.WithContext(ctx).First(&device, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finding device: %w", err)
	}
	return &device, nil
}

func (r *deviceRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Device, error) {
	var devices []models.Device
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND is_active = ?", userID, true).
		Find(&devices).Error
	if err != nil {
		return nil, fmt.Errorf("listing devices: %w", err)
	}
	return devices, nil
}

func (r *deviceRepository) CountByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.Device{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("counting devices: %w", err)
	}
	return count, nil
}

func (r *deviceRepository) Update(ctx context.Context, device *models.Device) error {
	if err := r.db.WithContext(ctx).Save(device).Error; err != nil {
		return fmt.Errorf("updating device: %w", err)
	}
	return nil
}

func (r *deviceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// soft-delete via is_active flag so the WireGuard public key is retained for audit
	err := r.db.WithContext(ctx).
		Model(&models.Device{}).
		Where("id = ?", id).
		Update("is_active", false).Error
	if err != nil {
		return fmt.Errorf("deleting device: %w", err)
	}
	return nil
}
