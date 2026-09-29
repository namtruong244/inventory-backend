package repository

import (
	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SettingsRepository interface {
	Create(settings *models.UserSettings) error
	GetByUserID(userID uuid.UUID) (*models.UserSettings, error)
	Update(settings *models.UserSettings) error
}

type settingsRepo struct {
	db *gorm.DB
}

func NewSettingsRepository(db *gorm.DB) SettingsRepository {
	return &settingsRepo{db: db}
}

func (r *settingsRepo) Create(settings *models.UserSettings) error {
	return r.db.Create(settings).Error
}

func (r *settingsRepo) GetByUserID(userID uuid.UUID) (*models.UserSettings, error) {
	var settings models.UserSettings
	err := r.db.Where("user_id = ?", userID).First(&settings).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create default settings if not exists
			defaultSettings := models.UserSettings{
				UserID:            userID,
				CurrencySymbol:    "$",
				DefaultUnit:       "pcs",
				ExpiryWarningDays: 30,
				NotifyExpiring:    true,
				NotifyOverdue:     true,
				NotifyLowStock:    true,
				VibrateOnScan:     true,
				AutoOpenOnScan:    true,
			}
			if createErr := r.db.Create(&defaultSettings).Error; createErr == nil {
				return &defaultSettings, nil
			}
		}
		return nil, err
	}
	return &settings, nil
}

func (r *settingsRepo) Update(settings *models.UserSettings) error {
	return r.db.Save(settings).Error
}
