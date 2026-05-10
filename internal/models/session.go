package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Session struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"user_id"`
	DeviceID      uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"device_id"`
	NodeID        uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"node_id"`
	TransportType string     `gorm:"not null"                                       json:"transport_type"` // xray, hysteria2, wireguard
	Protocol      string     `                                                      json:"protocol"`       // vless, vmess, etc.
	ClientIP      string     `                                                      json:"client_ip"`
	BytesIn       int64      `gorm:"default:0"                                      json:"bytes_in"`
	BytesOut      int64      `gorm:"default:0"                                      json:"bytes_out"`
	StartedAt     time.Time  `gorm:"not null"                                       json:"started_at"`
	EndedAt       *time.Time `                                                      json:"ended_at"`
	CreatedAt     time.Time  `                                                      json:"created_at"`

	User   *User   `gorm:"foreignKey:UserID"   json:"-"`
	Device *Device `gorm:"foreignKey:DeviceID" json:"-"`
	Node   *Node   `gorm:"foreignKey:NodeID"   json:"-"`
}

func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// Duration returns session duration; 0 if still active.
func (s *Session) Duration() time.Duration {
	if s.EndedAt == nil {
		return 0
	}
	return s.EndedAt.Sub(s.StartedAt)
}

// RefreshToken stores JWT refresh tokens for revocation support.
type RefreshToken struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index"                       json:"user_id"`
	TokenHash string         `gorm:"uniqueIndex;not null"                           json:"-"`
	DeviceID  string         `                                                      json:"device_id"`
	ExpiresAt time.Time      `gorm:"not null"                                       json:"expires_at"`
	RevokedAt *time.Time     `                                                      json:"revoked_at"`
	CreatedAt time.Time      `                                                      json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`
}

func (r *RefreshToken) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

func (r *RefreshToken) IsValid() bool {
	return r.RevokedAt == nil && time.Now().Before(r.ExpiresAt)
}
