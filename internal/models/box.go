package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Box struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"userId"`
	ParentID    *uuid.UUID `gorm:"type:uuid;index" json:"parentId"`
	Name        string     `gorm:"size:150;not null" json:"name"`
	Type        string     `gorm:"size:30;not null" json:"type"`
	Description string     `gorm:"type:text;default:''" json:"description"`
	Label       string     `gorm:"size:100;not null;index" json:"label"`
	Icon        *string    `gorm:"size:50" json:"icon,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`

	// Virtual / computed response fields
	ItemCount   int    `gorm:"-" json:"itemCount"`
	SubBoxCount int    `gorm:"-" json:"subBoxCount"`
	Children    []*Box `gorm:"-" json:"children,omitempty"`
}

type BreadcrumbItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (b *Box) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return
}
