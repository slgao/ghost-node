package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PlanType string

const (
	PlanFree    PlanType = "free"
	PlanBasic   PlanType = "basic"
	PlanPro     PlanType = "pro"
	PlanUnlimited PlanType = "unlimited"
)

type Subscription struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex"                 json:"user_id"`
	Plan             PlanType       `gorm:"type:varchar(30);default:'free'"                json:"plan"`
	BandwidthQuota   int64          `gorm:"default:10737418240"                            json:"bandwidth_quota"` // bytes; default 10 GB
	BandwidthUsed    int64          `gorm:"default:0"                                      json:"bandwidth_used"`
	MaxDevices       int            `gorm:"default:3"                                      json:"max_devices"`
	ExpiresAt        *time.Time     `                                                      json:"expires_at"`
	IsActive         bool           `gorm:"default:true"                                   json:"is_active"`
	CreatedAt        time.Time      `                                                      json:"created_at"`
	UpdatedAt        time.Time      `                                                      json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index"                                          json:"-"`

	User *User `gorm:"foreignKey:UserID" json:"-"`
}

func (s *Subscription) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (s *Subscription) RemainingBytes() int64 {
	return s.BandwidthQuota - s.BandwidthUsed
}

func (s *Subscription) IsExpired() bool {
	if s.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*s.ExpiresAt)
}
