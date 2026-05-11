package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/vpnplatform/core/internal/auth"
	"github.com/vpnplatform/core/internal/metrics"
	"github.com/vpnplatform/core/internal/models"
	"github.com/vpnplatform/core/internal/repository"
	"github.com/vpnplatform/core/pkg/crypto"
	"github.com/vpnplatform/core/pkg/logger"
)

type AuthService struct {
	userRepo repository.UserRepository
	jwtSvc   *auth.JWTService
	db       *gorm.DB
}

func NewAuthService(userRepo repository.UserRepository, jwtSvc *auth.JWTService, db *gorm.DB) *AuthService {
	return &AuthService{userRepo: userRepo, jwtSvc: jwtSvc, db: db}
}

type RegisterInput struct {
	Email    string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*models.User, *auth.TokenPair, error) {
	existing, err := s.userRepo.FindByEmail(ctx, in.Email)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, nil, fmt.Errorf("checking existing user: %w", err)
	}
	if existing != nil {
		return nil, nil, errors.New("email already registered")
	}

	hash, err := crypto.HashPassword(in.Password)
	if err != nil {
		return nil, nil, fmt.Errorf("hashing password: %w", err)
	}

	user := &models.User{
		Email:        in.Email,
		PasswordHash: hash,
		Role:         models.RoleUser,
		IsActive:     true,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, nil, fmt.Errorf("creating user: %w", err)
	}

	// create default free subscription
	sub := &models.Subscription{
		UserID:         user.ID,
		Plan:           models.PlanFree,
		BandwidthQuota: 10 * 1024 * 1024 * 1024, // 10 GB
		MaxDevices:     3,
	}
	if err := s.db.WithContext(ctx).Create(sub).Error; err != nil {
		logger.L().Warn("failed to create default subscription", zap.Error(err))
	}

	tokens, err := s.jwtSvc.GenerateTokenPair(user)
	if err != nil {
		return nil, nil, fmt.Errorf("generating tokens: %w", err)
	}

	if err := s.storeRefreshToken(ctx, user.ID, tokens.RefreshToken); err != nil {
		logger.L().Warn("failed to store refresh token", zap.Error(err))
	}

	metrics.RecordAuthEvent("register", true)
	metrics.RegisteredUsers.Inc()
	return user, tokens, nil
}

func (s *AuthService) Login(ctx context.Context, in LoginInput) (*auth.TokenPair, error) {
	user, err := s.userRepo.FindByEmail(ctx, in.Email)
	if errors.Is(err, repository.ErrNotFound) {
		metrics.RecordAuthEvent("login", false)
		return nil, errors.New("invalid credentials")
	}
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}

	if !user.IsActive {
		metrics.RecordAuthEvent("login", false)
		return nil, errors.New("account disabled")
	}

	if err := crypto.CheckPassword(user.PasswordHash, in.Password); err != nil {
		metrics.RecordAuthEvent("login", false)
		return nil, errors.New("invalid credentials")
	}

	tokens, err := s.jwtSvc.GenerateTokenPair(user)
	if err != nil {
		return nil, fmt.Errorf("generating tokens: %w", err)
	}

	if err := s.storeRefreshToken(ctx, user.ID, tokens.RefreshToken); err != nil {
		logger.L().Warn("failed to store refresh token", zap.Error(err))
	}

	metrics.RecordAuthEvent("login", true)
	return tokens, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	userID, err := s.jwtSvc.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	hash := hashToken(refreshToken)
	var stored models.RefreshToken
	if err := s.db.WithContext(ctx).
		Where("token_hash = ? AND user_id = ?", hash, userID).
		First(&stored).Error; err != nil {
		return nil, errors.New("refresh token not found or revoked")
	}

	if !stored.IsValid() {
		return nil, errors.New("refresh token expired or revoked")
	}

	// revoke old token
	now := time.Now()
	stored.RevokedAt = &now
	_ = s.db.WithContext(ctx).Save(&stored)

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}

	tokens, err := s.jwtSvc.GenerateTokenPair(user)
	if err != nil {
		return nil, fmt.Errorf("generating tokens: %w", err)
	}

	if err := s.storeRefreshToken(ctx, user.ID, tokens.RefreshToken); err != nil {
		logger.L().Warn("failed to store new refresh token", zap.Error(err))
	}

	return tokens, nil
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	hash := hashToken(refreshToken)
	now := time.Now()
	return s.db.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("token_hash = ?", hash).
		Update("revoked_at", now).Error
}

func (s *AuthService) storeRefreshToken(ctx context.Context, userID uuid.UUID, token string) error {
	rt := &models.RefreshToken{
		UserID:    userID,
		TokenHash: hashToken(token),
		ExpiresAt: time.Now().Add(s.jwtSvc.RefreshTTL()),
	}
	return s.db.WithContext(ctx).Create(rt).Error
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}
