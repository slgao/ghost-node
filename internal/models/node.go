package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
	NodeStatusDraining NodeStatus = "draining"
)

type Node struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name            string         `gorm:"not null"                                       json:"name"`
	Address         string         `gorm:"not null"                                       json:"address"`  // host or IP
	Region          string         `gorm:"not null"                                       json:"region"`
	Country         string         `                                                      json:"country"`
	Status          NodeStatus     `gorm:"type:varchar(20);default:'offline'"             json:"status"`
	LastHeartbeat   *time.Time     `                                                      json:"last_heartbeat"`
	AgentVersion    string         `                                                      json:"agent_version"`
	CPUUsage        float64        `                                                      json:"cpu_usage"`
	MemUsage        float64        `                                                      json:"mem_usage"`
	BandwidthIn     int64          `gorm:"default:0"                                      json:"bandwidth_in"`
	BandwidthOut    int64          `gorm:"default:0"                                      json:"bandwidth_out"`
	ActiveConns     int            `gorm:"default:0"                                      json:"active_conns"`
	IsPublic        bool           `gorm:"default:true"                                   json:"is_public"`
	CreatedAt       time.Time      `                                                      json:"created_at"`
	UpdatedAt       time.Time      `                                                      json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index"                                          json:"-"`

	TransportProfiles []TransportProfile `gorm:"foreignKey:NodeID" json:"transport_profiles,omitempty"`
}

func (n *Node) BeforeCreate(tx *gorm.DB) error {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	return nil
}
