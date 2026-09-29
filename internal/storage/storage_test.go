package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"inventory_backend/internal/config"
	"inventory_backend/internal/storage"
)

func TestLocalStorageProvider(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	provider := storage.NewLocalStorageProvider(tempDir, "https://api.example.com")

	dummyContent := "hello storage test"
	reader := strings.NewReader(dummyContent)

	url, err := provider.Upload(context.Background(), reader, "test.txt", "text/plain", "items")
	if err != nil {
		t.Fatalf("Expected successful upload, got err: %v", err)
	}

	expectedURL := "https://api.example.com/uploads/items/test.txt"
	if url != expectedURL {
		t.Errorf("Expected URL %q, got %q", expectedURL, url)
	}

	savedFile := filepath.Join(tempDir, "items", "test.txt")
	content, err := os.ReadFile(savedFile)
	if err != nil {
		t.Fatalf("Saved file could not be read: %v", err)
	}

	if string(content) != dummyContent {
		t.Errorf("Expected content %q, got %q", dummyContent, string(content))
	}
}

func TestR2StorageProvider_MissingCredentials(t *testing.T) {
	cfg := &config.Config{
		StorageDriver: "r2",
	}

	_, err := storage.NewStorageProvider(cfg)
	if err == nil {
		t.Fatalf("Expected error when initializing R2 without credentials, got nil")
	}
}

func TestStorageProviderFactory_DefaultLocal(t *testing.T) {
	cfg := &config.Config{
		StorageDriver: "local",
		UploadDir:     "./uploads",
		BaseURL:       "http://localhost:8080",
	}

	p, err := storage.NewStorageProvider(cfg)
	if err != nil {
		t.Fatalf("Expected no error for local driver, got %v", err)
	}

	if _, ok := p.(*storage.LocalStorageProvider); !ok {
		t.Fatalf("Expected *storage.LocalStorageProvider, got %T", p)
	}
}
