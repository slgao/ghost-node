package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TransportType string

const (
	TransportXray      TransportType = "xray"
	TransportHysteria2 TransportType = "hysteria2"
	TransportWireGuard TransportType = "wireguard"
)

// JSONB is a helper type for storing arbitrary JSON in PostgreSQL.
type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("unsupported type for JSONB")
	}
	return json.Unmarshal(bytes, j)
}

// TransportProfile holds the configuration for a single transport on a node.
type TransportProfile struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	NodeID    uuid.UUID      `gorm:"type:uuid;not null;index"                       json:"node_id"`
	Type      TransportType  `gorm:"type:varchar(30);not null"                      json:"type"`
	Port      int            `gorm:"not null"                                       json:"port"`
	Config    JSONB          `gorm:"type:jsonb"                                     json:"config"` // transport-specific settings
	IsActive  bool           `gorm:"default:true"                                   json:"is_active"`
	Priority  int            `gorm:"default:100"                                    json:"priority"` // lower = preferred
	CreatedAt time.Time      `                                                      json:"created_at"`
	UpdatedAt time.Time      `                                                      json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	Node *Node `gorm:"foreignKey:NodeID" json:"-"`
}

func (t *TransportProfile) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}
