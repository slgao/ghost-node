package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type User struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email        string         `gorm:"uniqueIndex;not null"                           json:"email"`
	PasswordHash string         `gorm:"not null"                                       json:"-"`
	Role         Role           `gorm:"type:varchar(20);default:'user'"                json:"role"`
	IsActive     bool           `gorm:"default:true"                                   json:"is_active"`
	CreatedAt    time.Time      `                                                      json:"created_at"`
	UpdatedAt    time.Time      `                                                      json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index"                                          json:"-"`

	Subscription *Subscription `gorm:"foreignKey:UserID" json:"subscription,omitempty"`
	Devices      []Device      `gorm:"foreignKey:UserID" json:"devices,omitempty"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
