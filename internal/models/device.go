package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DeviceType string

const (
	DeviceDesktop DeviceType = "desktop"
	DeviceMobile  DeviceType = "mobile"
	DeviceRouter  DeviceType = "router"
)

type Device struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID      `gorm:"type:uuid;not null;index"                       json:"user_id"`
	Name       string         `gorm:"not null"                                       json:"name"`
	Type       DeviceType     `gorm:"type:varchar(20);default:'desktop'"             json:"type"`
	PublicKey  string         `gorm:"uniqueIndex"                                    json:"public_key"` // WireGuard public key
	LastSeenAt *time.Time     `                                                      json:"last_seen_at"`
	LastIP     string         `                                                      json:"last_ip"`
	IsActive   bool           `gorm:"default:true"                                   json:"is_active"`
	CreatedAt  time.Time      `                                                      json:"created_at"`
	UpdatedAt  time.Time      `                                                      json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index"                                          json:"-"`

	User *User `gorm:"foreignKey:UserID" json:"-"`
}

func (d *Device) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}
