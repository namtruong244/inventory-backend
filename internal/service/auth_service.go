package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"inventory_backend/internal/config"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/storage"
	"inventory_backend/internal/utils"

	"inventory_backend/internal/apperr"

	"github.com/google/uuid"
)

// NormalizeEmail is the canonical stored form of an email address.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// registerFailedMsg is deliberately the same for "already registered" and a
// lost unique-index race, so the endpoint cannot be used to list accounts.
const registerFailedMsg = "Could not create the account. If you already have one, try signing in."

type AuthService interface {
	Register(name, email, password string) (*AuthResult, error)
	Login(email, password string) (*AuthResult, error)
	LoginWithGoogle(email, name string, avatarURL *string, idToken *string) (*AuthResult, error)
	GetProfile(userID uuid.UUID) (*models.User, error)
	// Refresh issues a fresh token for an already-authenticated user.
	Refresh(userID uuid.UUID) (*AuthResult, error)
	// RevokeAllTokens invalidates every token issued so far (logout everywhere).
	RevokeAllTokens(userID uuid.UUID) error
	// DeleteAccount permanently removes the user and all their data.
	DeleteAccount(userID uuid.UUID) error
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
	files        storage.StorageProvider
}

// AuthOption customises optional dependencies of the auth service.
type AuthOption func(*authService)

// WithFileStorage lets DeleteAccount remove the user's uploaded media too.
func WithFileStorage(p storage.StorageProvider) AuthOption {
	return func(s *authService) { s.files = p }
}

func NewAuthService(userRepo repository.UserRepository, settingsRepo repository.SettingsRepository, cfg *config.Config, opts ...AuthOption) AuthService {
	s := &authService{
		userRepo:     userRepo,
		settingsRepo: settingsRepo,
		cfg:          cfg,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *authService) Register(name, email, password string) (*AuthResult, error) {
	email = NormalizeEmail(email)
	name = strings.TrimSpace(name)
	if email == "" || password == "" || name == "" {
		return nil, apperr.Invalid("name, email and password are required")
	}

	// Check existing user
	existing, _ := s.userRepo.GetByEmail(email)
	if existing != nil {
		return nil, apperr.Invalid(registerFailedMsg)
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
		if apperr.IsUnique(err) {
			return nil, apperr.Invalid(registerFailedMsg)
		}
		return nil, err
	}

	// Create initial default user settings
	_ = s.settingsRepo.Create(models.NewDefaultSettings(user.ID))

	token, err := utils.GenerateToken(user.ID, user.Email, user.Role, user.TokenVersion, s.cfg.JWTSecret, s.cfg.JWTExpirationHours)
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
	user, err := s.userRepo.GetByEmail(NormalizeEmail(email))
	if err != nil || user == nil {
		return nil, apperr.Invalid("invalid email or password")
	}

	if user.PasswordHash == nil || *user.PasswordHash == "" || !utils.CheckPasswordHash(password, *user.PasswordHash) {
		return nil, apperr.Invalid("invalid email or password")
	}

	token, err := utils.GenerateToken(user.ID, user.Email, user.Role, user.TokenVersion, s.cfg.JWTSecret, s.cfg.JWTExpirationHours)
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

const googleVerifyTimeout = 5 * time.Second

var googleHTTPClient = &http.Client{Timeout: googleVerifyTimeout}

type GoogleTokenInfo struct {
	Aud           string `json:"aud"`
	Azp           string `json:"azp"`
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// isEmailVerified handles tokeninfo returning email_verified as either bool or string.
func (g *GoogleTokenInfo) isEmailVerified() bool {
	switch v := g.EmailVerified.(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

func verifyGoogleToken(idToken, expectedClientID string) (*GoogleTokenInfo, error) {
	if expectedClientID == "" {
		return nil, errors.New("google login is not configured")
	}

	endpoint := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(idToken)
	resp, err := googleHTTPClient.Get(endpoint)
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

	if info.Aud != expectedClientID && info.Azp != expectedClientID {
		return nil, errors.New("google token audience mismatch")
	}
	if info.Email == "" || !info.isEmailVerified() {
		return nil, errors.New("google account email is not verified")
	}

	return &info, nil
}

// LoginWithGoogle authenticates strictly by the Google id token: the email in the
// request body is never trusted. Tokens prefixed "mock_" are accepted only when APP_ENV=test.
func (s *authService) LoginWithGoogle(email, name string, avatarURL *string, idToken *string) (*AuthResult, error) {
	if idToken == nil || *idToken == "" {
		return nil, apperr.Invalid("id_token is required")
	}

	if strings.HasPrefix(*idToken, "mock_") {
		if s.cfg.AppEnv != "test" {
			return nil, errors.New("invalid Google token")
		}
	} else {
		info, err := verifyGoogleToken(*idToken, s.cfg.GoogleClientID)
		if err != nil {
			log.Printf("google login: token verification failed: %v", err)
			return nil, apperr.Invalid("Google authentication failed")
		}
		email = info.Email
		if info.Name != "" && (name == "" || name == "Google User") {
			name = info.Name
		}
		if info.Picture != "" && (avatarURL == nil || *avatarURL == "") {
			avatarURL = &info.Picture
		}
	}

	email = NormalizeEmail(email)
	if email == "" {
		return nil, apperr.Invalid("email is required")
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
		// Register does not verify email ownership, so a local account may have
		// been created by someone else. Google just proved the real owner, so
		// drop the password: nobody can keep using the pre-registered credentials.
		user.PasswordHash = nil
		user.Provider = "google"
		if err := s.userRepo.Update(user); err != nil {
			return nil, err
		}
		// ...and any token issued to the pre-registrant dies with it.
		if err := s.userRepo.BumpTokenVersion(user.ID); err != nil {
			return nil, err
		}
		if fresh, err := s.userRepo.GetByID(user.ID); err == nil && fresh != nil {
			user = fresh
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

		_ = s.settingsRepo.Create(models.NewDefaultSettings(user.ID))
	}

	token, err := utils.GenerateToken(user.ID, user.Email, user.Role, user.TokenVersion, s.cfg.JWTSecret, s.cfg.JWTExpirationHours)
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

func (s *authService) Refresh(userID uuid.UUID) (*AuthResult, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	token, err := utils.GenerateToken(user.ID, user.Email, user.Role, user.TokenVersion, s.cfg.JWTSecret, s.cfg.JWTExpirationHours)
	if err != nil {
		return nil, err
	}
	return &AuthResult{Token: token, User: UserResponse{
		ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role,
		AvatarURL: user.AvatarURL, Provider: user.Provider,
	}}, nil
}

func (s *authService) RevokeAllTokens(userID uuid.UUID) error {
	return s.userRepo.BumpTokenVersion(userID)
}

func (s *authService) DeleteAccount(userID uuid.UUID) error {
	files, err := s.userRepo.DeleteAccount(userID)
	if err != nil {
		return err
	}
	if s.files == nil {
		return nil
	}
	// The account is already gone; leftover files are only logged.
	for _, f := range files {
		// Only the user's own files go: a URL in another user's folder is not theirs to delete.
		if storage.BelongsToOther(f, userID) {
			log.Printf("account delete: skipping %s, it belongs to another user", f)
			continue
		}
		if err := s.files.Delete(context.Background(), f); err != nil {
			log.Printf("account delete: could not remove %s: %v", f, err)
		}
	}
	return nil
}
