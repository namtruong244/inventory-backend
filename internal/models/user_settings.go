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
	// WarrantyWarningDays is how many days ahead a warranty end counts as "ending soon".
	WarrantyWarningDays int `gorm:"default:30" json:"warrantyWarningDays"`
	// Timezone is an IANA name (e.g. Asia/Ho_Chi_Minh) used to decide what "today" is.
	Timezone       string `gorm:"size:64;default:'UTC'" json:"timezone"`
	NotifyExpiring bool   `gorm:"default:true" json:"notifyExpiring"`
	NotifyOverdue  bool   `gorm:"default:true" json:"notifyOverdue"`
	NotifyLowStock bool   `gorm:"default:true" json:"notifyLowStock"`
	VibrateOnScan  bool   `gorm:"default:true" json:"vibrateOnScan"`
	AutoOpenOnScan bool   `gorm:"default:true" json:"autoOpenOnScan"`
}

func (s *UserSettings) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return
}

// NewDefaultSettings returns the settings every new user starts with.
func NewDefaultSettings(userID uuid.UUID) *UserSettings {
	return &UserSettings{
		UserID:              userID,
		CurrencySymbol:      "$",
		DefaultUnit:         "pcs",
		ExpiryWarningDays:   30,
		WarrantyWarningDays: 30,
		Timezone:            "UTC",
		NotifyExpiring:      true,
		NotifyOverdue:       true,
		NotifyLowStock:      true,
		VibrateOnScan:       true,
		AutoOpenOnScan:      true,
	}
}
