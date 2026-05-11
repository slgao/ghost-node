package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vpnplatform/core/internal/metrics"
	"github.com/vpnplatform/core/internal/models"
	"github.com/vpnplatform/core/internal/repository"
)

type TrafficService struct {
	trafficRepo repository.TrafficRepository
	db          *gorm.DB
}

func NewTrafficService(trafficRepo repository.TrafficRepository, db *gorm.DB) *TrafficService {
	return &TrafficService{trafficRepo: trafficRepo, db: db}
}

type UsageSummary struct {
	Plan          models.PlanType
	QuotaBytes    int64
	UsedBytes     int64
	ExpiresAt     *time.Time
	PeriodBytesIn  int64
	PeriodBytesOut int64
	Daily         []models.TrafficUsage
}

// RecordUsage persists per-user traffic and updates the running subscription total.
func (s *TrafficService) RecordUsage(ctx context.Context, userID uuid.UUID, bytesIn, bytesOut int64) error {
	if bytesIn == 0 && bytesOut == 0 {
		return nil
	}
	if err := s.trafficRepo.UpsertDaily(ctx, userID, bytesIn, bytesOut); err != nil {
		return err
	}
	total := bytesIn + bytesOut
	if err := s.db.WithContext(ctx).
		Model(&models.Subscription{}).
		Where("user_id = ?", userID).
		UpdateColumn("bandwidth_used", gorm.Expr("bandwidth_used + ?", total)).Error; err != nil {
		return fmt.Errorf("updating bandwidth_used: %w", err)
	}
	metrics.RecordTraffic(bytesIn, bytesOut)
	return nil
}

// GetSummary returns a 30-day usage summary for the user.
func (s *TrafficService) GetSummary(ctx context.Context, userID uuid.UUID) (*UsageSummary, error) {
	var sub models.Subscription
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&sub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			sub = models.Subscription{Plan: models.PlanFree, BandwidthQuota: 10 * 1024 * 1024 * 1024}
		} else {
			return nil, fmt.Errorf("fetching subscription: %w", err)
		}
	}

	daily, err := s.trafficRepo.GetSummary(ctx, userID, 30)
	if err != nil {
		return nil, err
	}

	since := time.Now().UTC().AddDate(0, -1, 0)
	totalIn, totalOut, err := s.trafficRepo.GetPeriodTotal(ctx, userID, since)
	if err != nil {
		return nil, err
	}

	return &UsageSummary{
		Plan:           sub.Plan,
		QuotaBytes:     sub.BandwidthQuota,
		UsedBytes:      sub.BandwidthUsed,
		ExpiresAt:      sub.ExpiresAt,
		PeriodBytesIn:  totalIn,
		PeriodBytesOut: totalOut,
		Daily:          daily,
	}, nil
}

// CheckQuota returns an error if the user has exceeded their bandwidth quota.
func (s *TrafficService) CheckQuota(ctx context.Context, userID uuid.UUID) error {
	var sub models.Subscription
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&sub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("fetching subscription: %w", err)
	}
	if sub.IsExpired() {
		return errors.New("subscription expired")
	}
	if sub.RemainingBytes() <= 0 {
		return errors.New("bandwidth quota exceeded — upgrade your plan")
	}
	return nil
}
