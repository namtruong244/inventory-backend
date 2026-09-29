package service

import (
	"errors"

	"inventory_backend/internal/config"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/utils"

	"github.com/google/uuid"
)

type AuthService interface {
	Register(name, email, password string) (*AuthResult, error)
	Login(email, password string) (*AuthResult, error)
	GetProfile(userID uuid.UUID) (*models.User, error)
}

type AuthResult struct {
	Token string       `json:"token"`
	User  UserResponse `json:"user"`
}

type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	AvatarURL *string   `json:"avatarUrl,omitempty"`
}

type authService struct {
	userRepo     repository.UserRepository
	settingsRepo repository.SettingsRepository
	cfg          *config.Config
}

func NewAuthService(userRepo repository.UserRepository, settingsRepo repository.SettingsRepository, cfg *config.Config) AuthService {
	return &authService{
		userRepo:     userRepo,
		settingsRepo: settingsRepo,
		cfg:          cfg,
	}
}

func (s *authService) Register(name, email, password string) (*AuthResult, error) {
	if email == "" || password == "" || name == "" {
		return nil, errors.New("name, email and password are required")
	}

	// Check existing user
	existing, _ := s.userRepo.GetByEmail(email)
	if existing != nil {
		return nil, errors.New("email is already registered")
	}

	hashedPassword, err := utils.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Name:         name,
		Email:        email,
		PasswordHash: hashedPassword,
		Role:         "Owner",
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	// Create initial default user settings
	_ = s.settingsRepo.Create(&models.UserSettings{
		UserID:            user.ID,
		CurrencySymbol:    "$",
		DefaultUnit:       "pcs",
		ExpiryWarningDays: 30,
		NotifyExpiring:    true,
		NotifyOverdue:     true,
		NotifyLowStock:    true,
		VibrateOnScan:     true,
		AutoOpenOnScan:    true,
	})

	token, err := utils.GenerateToken(user.ID, user.Email, user.Role, s.cfg.JWTSecret, s.cfg.JWTExpirationHours)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Token: token,
		User: UserResponse{
			ID:        user.ID,
			Name:      user.Name,
			Email:     user.Email,
			Role:      user.Role,
			AvatarURL: user.AvatarURL,
		},
	}, nil
}

func (s *authService) Login(email, password string) (*AuthResult, error) {
	user, err := s.userRepo.GetByEmail(email)
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	if !utils.CheckPasswordHash(password, user.PasswordHash) {
		return nil, errors.New("invalid email or password")
	}

	token, err := utils.GenerateToken(user.ID, user.Email, user.Role, s.cfg.JWTSecret, s.cfg.JWTExpirationHours)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Token: token,
		User: UserResponse{
			ID:        user.ID,
			Name:      user.Name,
			Email:     user.Email,
			Role:      user.Role,
			AvatarURL: user.AvatarURL,
		},
	}, nil
}

func (s *authService) GetProfile(userID uuid.UUID) (*models.User, error) {
	return s.userRepo.GetByID(userID)
}
