package service_test

import (
	"testing"

	"inventory_backend/internal/config"
	"inventory_backend/internal/models"
	"inventory_backend/internal/service"

	"github.com/google/uuid"
)

type mockUserRepo struct {
	users map[string]*models.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*models.User)}
}

func (m *mockUserRepo) Create(user *models.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	m.users[user.Email] = user
	return nil
}

func (m *mockUserRepo) GetByEmail(email string) (*models.User, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, nil
}

func (m *mockUserRepo) GetByID(id uuid.UUID) (*models.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockUserRepo) Update(user *models.User) error {
	m.users[user.Email] = user
	return nil
}

type mockSettingsRepo struct {
	settings map[uuid.UUID]*models.UserSettings
}

func newMockSettingsRepo() *mockSettingsRepo {
	return &mockSettingsRepo{settings: make(map[uuid.UUID]*models.UserSettings)}
}

func (m *mockSettingsRepo) Create(s *models.UserSettings) error {
	m.settings[s.UserID] = s
	return nil
}

func (m *mockSettingsRepo) GetByUserID(userID uuid.UUID) (*models.UserSettings, error) {
	if s, ok := m.settings[userID]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *mockSettingsRepo) Update(s *models.UserSettings) error {
	m.settings[s.UserID] = s
	return nil
}

func TestAuthService_GoogleAndAppleLogin(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:          "test-secret-key-12345",
		JWTExpirationHours: 24,
	}
	userRepo := newMockUserRepo()
	settingsRepo := newMockSettingsRepo()
	authSvc := service.NewAuthService(userRepo, settingsRepo, cfg)

	// Test 1: Register new user via Google
	avatarURL := "https://example.com/avatar.png"
	res, err := authSvc.LoginWithGoogle("googleuser@gmail.com", "Google User", &avatarURL)
	if err != nil {
		t.Fatalf("expected successful Google login, got: %v", err)
	}
	if res.User.Email != "googleuser@gmail.com" {
		t.Errorf("expected email googleuser@gmail.com, got: %s", res.User.Email)
	}
	if res.User.Provider != "google" {
		t.Errorf("expected provider google, got: %s", res.User.Provider)
	}
	if res.Token == "" {
		t.Errorf("expected non-empty JWT token")
	}

	// Verify settings were created
	settings, _ := settingsRepo.GetByUserID(res.User.ID)
	if settings == nil {
		t.Fatalf("expected user settings to be created")
	}
	if settings.CurrencySymbol != "$" {
		t.Errorf("expected default currency $, got: %s", settings.CurrencySymbol)
	}

	// Test 2: Logging in again via Google updates avatar if provided
	newAvatar := "https://example.com/new_avatar.png"
	res2, err := authSvc.LoginWithGoogle("googleuser@gmail.com", "Google User Updated", &newAvatar)
	if err != nil {
		t.Fatalf("expected successful Google re-login, got: %v", err)
	}
	if res2.User.ID != res.User.ID {
		t.Errorf("expected same user ID, got %s vs %s", res2.User.ID, res.User.ID)
	}
	if res2.User.AvatarURL == nil || *res2.User.AvatarURL != newAvatar {
		t.Errorf("expected updated avatar, got: %v", res2.User.AvatarURL)
	}

	// Test 3: Password login on social user does not crash and fails gracefully
	_, err = authSvc.Login("googleuser@gmail.com", "somepassword")
	if err == nil {
		t.Errorf("expected error for password login on social user without password, got nil")
	}

	// Test 4: Apple Login
	resApple, err := authSvc.LoginWithApple("appleuser@privaterelay.appleid.com", "Apple User")
	if err != nil {
		t.Fatalf("expected successful Apple login, got: %v", err)
	}
	if resApple.User.Provider != "apple" {
		t.Errorf("expected provider apple, got: %s", resApple.User.Provider)
	}
}
