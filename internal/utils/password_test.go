package utils_test

import (
	"testing"

	"inventory_backend/internal/utils"
)

func TestPasswordHashingAndChecking(t *testing.T) {
	password := "SecurePassword123!"

	hash, err := utils.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash == "" || hash == password {
		t.Fatal("expected hashed password to be non-empty and different from plain password")
	}

	if !utils.CheckPasswordHash(password, hash) {
		t.Error("expected correct password to match hash")
	}

	if utils.CheckPasswordHash("WrongPassword123!", hash) {
		t.Error("expected wrong password to not match hash")
	}
}
