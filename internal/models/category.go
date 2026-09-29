package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Category struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    *uuid.UUID `gorm:"type:uuid;index" json:"userId,omitempty"`
	Name      string     `gorm:"size:100;not null" json:"name"`
	IconName  string     `gorm:"size:50;default:'devices'" json:"iconName"`
	IsDefault bool       `gorm:"default:false" json:"isDefault"`
	CreatedAt time.Time  `json:"createdAt"`
}

func (c *Category) BeforeCreate(tx *gorm.DB) (err error) {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return
}
