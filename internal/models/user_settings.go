package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserSettings struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	UserID            uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"userId"`
	CurrencySymbol    string    `gorm:"size:10;default:'$'" json:"currencySymbol"`
	DefaultUnit       string    `gorm:"size:20;default:'pcs'" json:"defaultUnit"`
	ExpiryWarningDays int       `gorm:"default:30" json:"expiryWarningDays"`
	NotifyExpiring    bool      `gorm:"default:true" json:"notifyExpiring"`
	NotifyOverdue     bool      `gorm:"default:true" json:"notifyOverdue"`
	NotifyLowStock    bool      `gorm:"default:true" json:"notifyLowStock"`
	VibrateOnScan     bool      `gorm:"default:true" json:"vibrateOnScan"`
	AutoOpenOnScan    bool      `gorm:"default:true" json:"autoOpenOnScan"`
}

func (s *UserSettings) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return
}
