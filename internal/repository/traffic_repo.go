package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vpnplatform/core/internal/models"
)

type TrafficRepository interface {
	UpsertDaily(ctx context.Context, userID uuid.UUID, bytesIn, bytesOut int64) error
	GetSummary(ctx context.Context, userID uuid.UUID, days int) ([]models.TrafficUsage, error)
	GetPeriodTotal(ctx context.Context, userID uuid.UUID, since time.Time) (bytesIn, bytesOut int64, err error)
}

type trafficRepository struct {
	db *gorm.DB
}

func NewTrafficRepository(db *gorm.DB) TrafficRepository {
	return &trafficRepository{db: db}
}

func (r *trafficRepository) UpsertDaily(ctx context.Context, userID uuid.UUID, bytesIn, bytesOut int64) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)

	result := r.db.WithContext(ctx).
		Model(&models.TrafficUsage{}).
		Where("user_id = ? AND date = ?", userID, today).
		Updates(map[string]interface{}{
			"bytes_in":   gorm.Expr("bytes_in + ?", bytesIn),
			"bytes_out":  gorm.Expr("bytes_out + ?", bytesOut),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("updating daily traffic: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	record := &models.TrafficUsage{
		UserID:   userID,
		Date:     today,
		BytesIn:  bytesIn,
		BytesOut: bytesOut,
	}
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("creating daily traffic: %w", err)
	}
	return nil
}

func (r *trafficRepository) GetSummary(ctx context.Context, userID uuid.UUID, days int) ([]models.TrafficUsage, error) {
	since := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -days+1)
	var records []models.TrafficUsage
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND date >= ?", userID, since).
		Order("date DESC").
		Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("fetching traffic summary: %w", err)
	}
	return records, nil
}

func (r *trafficRepository) GetPeriodTotal(ctx context.Context, userID uuid.UUID, since time.Time) (int64, int64, error) {
	var totals struct {
		TotalIn  int64
		TotalOut int64
	}
	err := r.db.WithContext(ctx).
		Model(&models.TrafficUsage{}).
		Select("COALESCE(SUM(bytes_in), 0) as total_in, COALESCE(SUM(bytes_out), 0) as total_out").
		Where("user_id = ? AND date >= ?", userID, since).
		Scan(&totals).Error
	if err != nil {
		return 0, 0, fmt.Errorf("fetching period total: %w", err)
	}
	return totals.TotalIn, totals.TotalOut, nil
}
