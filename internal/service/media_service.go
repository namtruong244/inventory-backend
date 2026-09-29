package service

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"inventory_backend/internal/storage"

	"github.com/google/uuid"
)

type MediaService interface {
	Upload(file *multipart.FileHeader, folder string) (*MediaUploadResponse, error)
	UploadWithContext(ctx context.Context, file *multipart.FileHeader, folder string) (*MediaUploadResponse, error)
}

type MediaUploadResponse struct {
	URL string `json:"url"`
}

type mediaService struct {
	storageProvider storage.StorageProvider
}

func NewMediaService(storageProvider storage.StorageProvider) MediaService {
	return &mediaService{storageProvider: storageProvider}
}

func (s *mediaService) Upload(file *multipart.FileHeader, folder string) (*MediaUploadResponse, error) {
	return s.UploadWithContext(context.Background(), file, folder)
}

func (s *mediaService) UploadWithContext(ctx context.Context, file *multipart.FileHeader, folder string) (*MediaUploadResponse, error) {
	if folder != "items" && folder != "receipts" {
		folder = "items"
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedExtensions := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".webp": "image/webp",
		".gif":  "image/gif",
		".heic": "image/heic",
		".pdf":  "application/pdf",
	}

	defaultMime, allowed := allowedExtensions[ext]
	if !allowed {
		return nil, errors.New("unsupported file extension (allowed: jpg, jpeg, png, webp, gif, heic, pdf)")
	}

	contentType := file.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = defaultMime
	}

	uniqueFilename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), uuid.New().String()[:8], ext)

	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	fileURL, err := s.storageProvider.Upload(ctx, src, uniqueFilename, contentType, folder)
	if err != nil {
		return nil, fmt.Errorf("failed to upload media: %w", err)
	}

	return &MediaUploadResponse{
		URL: fileURL,
	}, nil
}
