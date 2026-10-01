package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"inventory_backend/internal/config"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/utils"

	"github.com/google/uuid"
)

type AuthService interface {
	Register(name, email, password string) (*AuthResult, error)
	Login(email, password string) (*AuthResult, error)
	LoginWithGoogle(email, name string, avatarURL *string, idToken *string) (*AuthResult, error)
	LoginWithApple(email, name string, identityToken *string) (*AuthResult, error)
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
	Provider  string    `json:"provider,omitempty"`
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
		PasswordHash: &hashedPassword,
		Provider:     "local",
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
			Provider:  user.Provider,
		},
	}, nil
}

func (s *authService) Login(email, password string) (*AuthResult, error) {
	user, err := s.userRepo.GetByEmail(email)
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	if user.PasswordHash == nil || *user.PasswordHash == "" || !utils.CheckPasswordHash(password, *user.PasswordHash) {
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
			Provider:  user.Provider,
		},
	}, nil
}

type GoogleTokenInfo struct {
	Aud           string `json:"aud"`
	Azp           string `json:"azp"`
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func verifyGoogleToken(idToken, expectedClientID string) (*GoogleTokenInfo, error) {
	// Bypass verification for mock tokens in test/dev
	if strings.HasPrefix(idToken, "mock_") {
		return nil, nil
	}

	url := fmt.Sprintf("https://oauth2.googleapis.com/tokeninfo?id_token=%s", idToken)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to verify google token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("invalid or expired Google token")
	}

	var info GoogleTokenInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	// Verify audience matches expected Client ID
	if expectedClientID != "" && info.Aud != expectedClientID && info.Azp != expectedClientID {
		return nil, errors.New("google token audience mismatch")
	}

	return &info, nil
}

func (s *authService) LoginWithGoogle(email, name string, avatarURL *string, idToken *string) (*AuthResult, error) {
	// Verify idToken with Google if provided
	if idToken != nil && *idToken != "" && !strings.HasPrefix(*idToken, "mock_") {
		info, err := verifyGoogleToken(*idToken, s.cfg.GoogleClientID)
		if err != nil {
			return nil, fmt.Errorf("google authentication failed: %w", err)
		}
		if info != nil {
			if info.Email != "" {
				email = info.Email
			}
			if info.Name != "" && (name == "" || name == "Google User") {
				name = info.Name
			}
			if info.Picture != "" && (avatarURL == nil || *avatarURL == "") {
				avatarURL = &info.Picture
			}
		}
	}

	if email == "" {
		return nil, errors.New("email is required")
	}
	if name == "" {
		name = "Google User"
	}

	user, _ := s.userRepo.GetByEmail(email)
	if user != nil {
		// Existing user: update avatar if provided
		if avatarURL != nil && *avatarURL != "" {
			user.AvatarURL = avatarURL
		}
		if user.Name == "" || user.Name == "Google User" {
			user.Name = name
		}
		user.Provider = "google"
		if err := s.userRepo.Update(user); err != nil {
			return nil, err
		}
	} else {
		// New user
		user = &models.User{
			Name:         name,
			Email:        email,
			Provider:     "google",
			Role:         "Owner",
			AvatarURL:    avatarURL,
			PasswordHash: nil,
		}
		if err := s.userRepo.Create(user); err != nil {
			return nil, err
		}

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
			Provider:  user.Provider,
		},
	}, nil
}

func (s *authService) LoginWithApple(email, name string, identityToken *string) (*AuthResult, error) {
	if email == "" {
		return nil, errors.New("email is required")
	}
	if name == "" {
		name = "Apple User"
	}

	user, _ := s.userRepo.GetByEmail(email)
	if user != nil {
		user.Provider = "apple"
		// Do not overwrite existing real name with default "Apple User"
		if (user.Name == "" || user.Name == "Apple User") && name != "Apple User" {
			user.Name = name
		}
		if err := s.userRepo.Update(user); err != nil {
			return nil, err
		}
	} else {
		user = &models.User{
			Name:         name,
			Email:        email,
			Provider:     "apple",
			Role:         "Owner",
			PasswordHash: nil,
		}
		if err := s.userRepo.Create(user); err != nil {
			return nil, err
		}

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
			Provider:  user.Provider,
		},
	}, nil
}

func (s *authService) GetProfile(userID uuid.UUID) (*models.User, error) {
	return s.userRepo.GetByID(userID)
}
