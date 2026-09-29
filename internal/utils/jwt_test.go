package utils_test

import (
	"testing"

	"inventory_backend/internal/utils"

	"github.com/google/uuid"
)

func TestJWTGenerationAndValidation(t *testing.T) {
	secret := "test_secret_key_1234567890_abcdef"
	userID := uuid.New()
	email := "user@example.com"
	role := "Owner"

	token, err := utils.GenerateToken(userID, email, role, secret, 24)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := utils.ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, claims.UserID)
	}
	if claims.Email != email {
		t.Errorf("expected email %s, got %s", email, claims.Email)
	}
	if claims.Role != role {
		t.Errorf("expected role %s, got %s", role, claims.Role)
	}

	// Validate with wrong secret
	_, err = utils.ValidateToken(token, "wrong_secret_key")
	if err == nil {
		t.Error("expected error when validating with wrong secret, got nil")
	}
}
