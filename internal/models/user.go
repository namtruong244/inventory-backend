package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name         string    `gorm:"size:150;not null" json:"name"`
	Email        string    `gorm:"size:255;uniqueIndex;not null" json:"email"`
	PasswordHash *string   `gorm:"size:255" json:"-"`
	Provider     string    `gorm:"size:50;default:'local'" json:"provider"`
	ProviderID   *string   `gorm:"size:255" json:"providerId,omitempty"`
	Role         string    `gorm:"size:50;default:'Owner'" json:"role"`
	AvatarURL    *string   `gorm:"type:text" json:"avatarUrl,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	if u.Provider == "" {
		u.Provider = "local"
	}
	if u.Role == "" {
		u.Role = "Owner"
	}
	return
}
