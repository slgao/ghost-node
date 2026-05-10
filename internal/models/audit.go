package models

import (
	"time"

	"github.com/google/uuid"
)

type AuditAction string

const (
	AuditLogin        AuditAction = "auth.login"
	AuditLogout       AuditAction = "auth.logout"
	AuditRegister     AuditAction = "auth.register"
	AuditConnect      AuditAction = "vpn.connect"
	AuditDisconnect   AuditAction = "vpn.disconnect"
	AuditDeviceAdd    AuditAction = "device.add"
	AuditDeviceRemove AuditAction = "device.remove"
	AuditNodeCreate   AuditAction = "node.create"
	AuditNodeDelete   AuditAction = "node.delete"
)

type AuditLog struct {
	ID         uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     *uuid.UUID  `gorm:"type:uuid;index"                               json:"user_id"`
	Action     AuditAction `gorm:"not null"                                      json:"action"`
	Resource   string      `                                                     json:"resource"` // e.g. "node:uuid"
	Details    JSONB       `gorm:"type:jsonb"                                    json:"details"`
	IPAddress  string      `                                                     json:"ip_address"`
	UserAgent  string      `                                                     json:"user_agent"`
	Success    bool        `gorm:"default:true"                                  json:"success"`
	CreatedAt  time.Time   `                                                     json:"created_at"`
}

func (a *AuditLog) BeforeCreate(_ interface{}) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

// TrafficUsage records daily per-user bandwidth consumption.
type TrafficUsage struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index:idx_traffic_user_date"  json:"user_id"`
	Date      time.Time `gorm:"type:date;not null;index:idx_traffic_user_date"  json:"date"`
	BytesIn   int64     `gorm:"default:0"                                      json:"bytes_in"`
	BytesOut  int64     `gorm:"default:0"                                      json:"bytes_out"`
	CreatedAt time.Time `                                                      json:"created_at"`
	UpdatedAt time.Time `                                                      json:"updated_at"`
}

func (t *TrafficUsage) BeforeCreate(_ interface{}) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}
