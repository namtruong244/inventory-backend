package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"inventory_backend/internal/config"
	"inventory_backend/internal/handler"
	"inventory_backend/internal/models"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type mockUserRepo struct {
	users map[string]*models.User
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

type mockSettingsRepo struct{}

func (m *mockSettingsRepo) Create(s *models.UserSettings) error {
	return nil
}

func (m *mockSettingsRepo) GetByUserID(userID uuid.UUID) (*models.UserSettings, error) {
	return &models.UserSettings{UserID: userID, CurrencySymbol: "$"}, nil
}

func (m *mockSettingsRepo) Update(s *models.UserSettings) error {
	return nil
}

func setupTestRouter() (*gin.Engine, *handler.AuthHandler) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		JWTSecret:          "test-secret",
		JWTExpirationHours: 24,
	}
	userRepo := &mockUserRepo{users: make(map[string]*models.User)}
	settingsRepo := &mockSettingsRepo{}
	authService := service.NewAuthService(userRepo, settingsRepo, cfg)
	authHandler := handler.NewAuthHandler(authService)

	r := gin.New()
	v1 := r.Group("/api/v1/auth")
	{
		v1.POST("/google", authHandler.GoogleLogin)
		v1.POST("/apple", authHandler.AppleLogin)
	}
	return r, authHandler
}

func TestGoogleLoginEndpoint(t *testing.T) {
	r, _ := setupTestRouter()

	body := map[string]interface{}{
		"email":      "testgoogle@gmail.com",
		"name":       "Test Google",
		"avatar_url": "https://example.com/avatar.jpg",
	}
	raw, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/google", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got: %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Token string `json:"token"`
			User  struct {
				Email    string `json:"email"`
				Provider string `json:"provider"`
			} `json:"user"`
		} `json:"data"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success || resp.Data.Token == "" {
		t.Errorf("expected success with non-empty token")
	}
	if resp.Data.User.Provider != "google" {
		t.Errorf("expected provider google, got: %s", resp.Data.User.Provider)
	}
}

func TestAppleLoginEndpoint(t *testing.T) {
	r, _ := setupTestRouter()

	body := map[string]interface{}{
		"email": "appleuser@privaterelay.appleid.com",
		"name":  "Apple User",
	}
	raw, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/apple", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got: %d, body: %s", w.Code, w.Body.String())
	}
}
