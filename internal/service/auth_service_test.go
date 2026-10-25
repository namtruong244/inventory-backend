package service_test

import (
	"strings"
	"testing"

	"inventory_backend/internal/config"
	"inventory_backend/internal/models"
	"inventory_backend/internal/service"

	"github.com/google/uuid"
	"gorm.io/gorm"
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

func (m *mockUserRepo) BumpTokenVersion(id uuid.UUID) error {
	for _, u := range m.users {
		if u.ID == id {
			u.TokenVersion++
		}
	}
	return nil
}

func (m *mockUserRepo) DeleteAccount(id uuid.UUID) ([]string, error) {
	for email, u := range m.users {
		if u.ID == id {
			delete(m.users, email)
			return nil, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
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

func TestAuthService_GoogleLogin(t *testing.T) {
	cfg := &config.Config{
		AppEnv:             "test",
		JWTSecret:          "test-secret-key-12345",
		JWTExpirationHours: 24,
	}
	userRepo := newMockUserRepo()
	settingsRepo := newMockSettingsRepo()
	authSvc := service.NewAuthService(userRepo, settingsRepo, cfg)

	// Test 1: Register new user via Google
	avatarURL := "https://example.com/avatar.png"
	mockIDToken := "mock_google_id_token_123"
	res, err := authSvc.LoginWithGoogle("googleuser@gmail.com", "Google User", &avatarURL, &mockIDToken)
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
	res2, err := authSvc.LoginWithGoogle("googleuser@gmail.com", "Google User Updated", &newAvatar, &mockIDToken)
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
}

func TestAuthService_GoogleLoginRejectsUnverified(t *testing.T) {
	newSvc := func(env string) service.AuthService {
		cfg := &config.Config{AppEnv: env, JWTSecret: "s", JWTExpirationHours: 1, GoogleClientID: "client"}
		return service.NewAuthService(newMockUserRepo(), newMockSettingsRepo(), cfg)
	}

	// Missing id token must never fall back to trusting the body email
	if _, err := newSvc("test").LoginWithGoogle("victim@gmail.com", "Victim", nil, nil); err == nil {
		t.Errorf("expected error when id_token is missing")
	}
	empty := ""
	if _, err := newSvc("test").LoginWithGoogle("victim@gmail.com", "Victim", nil, &empty); err == nil {
		t.Errorf("expected error when id_token is empty")
	}

	// mock_ tokens are only honoured in the test environment
	mock := "mock_token"
	for _, env := range []string{"production", "development", ""} {
		if _, err := newSvc(env).LoginWithGoogle("victim@gmail.com", "Victim", nil, &mock); err == nil {
			t.Errorf("expected mock token to be rejected in env %q", env)
		}
	}
}

func TestAuthService_GoogleLinkDropsPreRegisteredPassword(t *testing.T) {
	cfg := &config.Config{AppEnv: "test", JWTSecret: "s", JWTExpirationHours: 1}
	svc := service.NewAuthService(newMockUserRepo(), newMockSettingsRepo(), cfg)

	if _, err := svc.Register("Attacker", "victim@example.com", "attackerpass1"); err != nil {
		t.Fatal(err)
	}
	tok := "mock_token"
	if _, err := svc.LoginWithGoogle("victim@example.com", "Victim", nil, &tok); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login("victim@example.com", "attackerpass1"); err == nil {
		t.Fatal("pre-registered password must stop working after Google link")
	}
}

func TestAuthService_DeleteAccount(t *testing.T) {
	cfg := &config.Config{AppEnv: "test", JWTSecret: "s", JWTExpirationHours: 1}
	svc := service.NewAuthService(newMockUserRepo(), newMockSettingsRepo(), cfg)

	res, err := svc.Register("U", "u@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteAccount(res.User.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Login("u@example.com", "password123"); err == nil {
		t.Fatal("login must fail after account deletion")
	}
}

func TestAuthService_EmailIsNormalized(t *testing.T) {
	cfg := &config.Config{AppEnv: "test", JWTSecret: "s", JWTExpirationHours: 1}
	svc := service.NewAuthService(newMockUserRepo(), newMockSettingsRepo(), cfg)

	res, err := svc.Register("U", "  User@Example.COM ", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if res.User.Email != "user@example.com" {
		t.Fatalf("stored email = %q", res.User.Email)
	}
	if _, err := svc.Login("USER@example.com", "password123"); err != nil {
		t.Fatalf("login with different case: %v", err)
	}
	_, err = svc.Register("Other", "user@EXAMPLE.com", "password123")
	if err == nil {
		t.Fatal("registering the same email in another case must fail")
	}
	// The message must not confirm that the address exists.
	if strings.Contains(strings.ToLower(err.Error()), "already registered") {
		t.Fatalf("register error leaks account existence: %v", err)
	}
}
