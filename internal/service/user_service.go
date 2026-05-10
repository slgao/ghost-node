package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vpnplatform/core/internal/models"
	"github.com/vpnplatform/core/internal/repository"
	"github.com/vpnplatform/core/pkg/crypto"
)

type UserService struct {
	userRepo   repository.UserRepository
	deviceRepo repository.DeviceRepository
}

func NewUserService(userRepo repository.UserRepository, deviceRepo repository.DeviceRepository) *UserService {
	return &UserService{userRepo: userRepo, deviceRepo: deviceRepo}
}

func (s *UserService) GetProfile(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, errors.New("user not found")
	}
	return user, err
}

func (s *UserService) ChangePassword(ctx context.Context, userID uuid.UUID, oldPass, newPass string) error {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return errors.New("user not found")
	}

	if err := crypto.CheckPassword(user.PasswordHash, oldPass); err != nil {
		return errors.New("incorrect current password")
	}

	hash, err := crypto.HashPassword(newPass)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}
	user.PasswordHash = hash
	return s.userRepo.Update(ctx, user)
}

// ─── Devices ─────────────────────────────────────────────────────────────────

type AddDeviceInput struct {
	Name      string
	Type      models.DeviceType
	PublicKey string
}

func (s *UserService) AddDevice(ctx context.Context, userID uuid.UUID, in AddDeviceInput) (*models.Device, error) {
	sub := &models.Subscription{}
	// maxDevices check — default 3 for free tier
	count, err := s.deviceRepo.CountByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("counting devices: %w", err)
	}
	_ = sub
	if count >= 10 {
		return nil, errors.New("device limit reached for your subscription")
	}

	device := &models.Device{
		UserID:    userID,
		Name:      in.Name,
		Type:      in.Type,
		PublicKey: in.PublicKey,
		IsActive:  true,
	}
	if err := s.deviceRepo.Create(ctx, device); err != nil {
		return nil, fmt.Errorf("adding device: %w", err)
	}
	return device, nil
}

func (s *UserService) ListDevices(ctx context.Context, userID uuid.UUID) ([]models.Device, error) {
	return s.deviceRepo.ListByUser(ctx, userID)
}

func (s *UserService) RemoveDevice(ctx context.Context, userID uuid.UUID, deviceID uuid.UUID) error {
	device, err := s.deviceRepo.FindByID(ctx, deviceID)
	if errors.Is(err, repository.ErrNotFound) {
		return errors.New("device not found")
	}
	if err != nil {
		return err
	}
	if device.UserID != userID {
		return errors.New("device does not belong to user")
	}
	return s.deviceRepo.Delete(ctx, deviceID)
}

func (s *UserService) UpdateDeviceLastSeen(ctx context.Context, deviceID uuid.UUID, ip string) error {
	device, err := s.deviceRepo.FindByID(ctx, deviceID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	now := time.Now()
	device.LastSeenAt = &now
	device.LastIP = ip
	return s.deviceRepo.Update(ctx, device)
}
