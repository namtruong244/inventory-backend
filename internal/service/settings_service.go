package service

import (
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"

	"github.com/google/uuid"
)

type SettingsService interface {
	GetSettings(userID uuid.UUID) (*models.UserSettings, error)
	UpdateSettings(userID uuid.UUID, req UpdateSettingsRequest) (*models.UserSettings, error)
	UpdateProfile(userID uuid.UUID, req UpdateProfileRequest) (*models.User, error)
}

type UpdateSettingsRequest struct {
	CurrencySymbol    *string `json:"currencySymbol"`
	DefaultUnit       *string `json:"defaultUnit"`
	ExpiryWarningDays *int    `json:"expiryWarningDays"`
	NotifyExpiring    *bool   `json:"notifyExpiring"`
	NotifyOverdue     *bool   `json:"notifyOverdue"`
	NotifyLowStock    *bool   `json:"notifyLowStock"`
	VibrateOnScan     *bool   `json:"vibrateOnScan"`
	AutoOpenOnScan    *bool   `json:"autoOpenOnScan"`
}

type UpdateProfileRequest struct {
	Name      *string `json:"name"`
	Role      *string `json:"role"`
	AvatarURL *string `json:"avatarUrl"`
}

type settingsService struct {
	settingsRepo repository.SettingsRepository
	userRepo     repository.UserRepository
}

func NewSettingsService(settingsRepo repository.SettingsRepository, userRepo repository.UserRepository) SettingsService {
	return &settingsService{
		settingsRepo: settingsRepo,
		userRepo:     userRepo,
	}
}

func (s *settingsService) GetSettings(userID uuid.UUID) (*models.UserSettings, error) {
	return s.settingsRepo.GetByUserID(userID)
}

func (s *settingsService) UpdateSettings(userID uuid.UUID, req UpdateSettingsRequest) (*models.UserSettings, error) {
	settings, err := s.settingsRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	if req.CurrencySymbol != nil {
		settings.CurrencySymbol = *req.CurrencySymbol
	}
	if req.DefaultUnit != nil {
		settings.DefaultUnit = *req.DefaultUnit
	}
	if req.ExpiryWarningDays != nil {
		settings.ExpiryWarningDays = *req.ExpiryWarningDays
	}
	if req.NotifyExpiring != nil {
		settings.NotifyExpiring = *req.NotifyExpiring
	}
	if req.NotifyOverdue != nil {
		settings.NotifyOverdue = *req.NotifyOverdue
	}
	if req.NotifyLowStock != nil {
		settings.NotifyLowStock = *req.NotifyLowStock
	}
	if req.VibrateOnScan != nil {
		settings.VibrateOnScan = *req.VibrateOnScan
	}
	if req.AutoOpenOnScan != nil {
		settings.AutoOpenOnScan = *req.AutoOpenOnScan
	}

	if err := s.settingsRepo.Update(settings); err != nil {
		return nil, err
	}

	return settings, nil
}

func (s *settingsService) UpdateProfile(userID uuid.UUID, req UpdateProfileRequest) (*models.User, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil && *req.Name != "" {
		user.Name = *req.Name
	}
	if req.Role != nil && *req.Role != "" {
		user.Role = *req.Role
	}
	if req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	return user, nil
}
