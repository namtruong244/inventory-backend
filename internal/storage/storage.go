package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"inventory_backend/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// StorageProvider defines the contract for uploading media assets.
type StorageProvider interface {
	Upload(ctx context.Context, file io.Reader, filename string, contentType string, folder string) (string, error)
}

// LocalStorageProvider saves files to the local filesystem.
type LocalStorageProvider struct {
	uploadDir string
	baseURL   string
}

func NewLocalStorageProvider(uploadDir, baseURL string) *LocalStorageProvider {
	return &LocalStorageProvider{
		uploadDir: uploadDir,
		baseURL:   baseURL,
	}
}

func (l *LocalStorageProvider) Upload(ctx context.Context, file io.Reader, filename string, contentType string, folder string) (string, error) {
	targetDir := filepath.Join(l.uploadDir, folder)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create upload directory: %w", err)
	}

	targetPath := filepath.Join(targetDir, filename)
	dst, err := os.Create(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	if _, err = io.Copy(dst, file); err != nil {
		return "", fmt.Errorf("failed to save file locally: %w", err)
	}

	url := fmt.Sprintf("%s/uploads/%s/%s", strings.TrimRight(l.baseURL, "/"), folder, filename)
	return url, nil
}

// R2StorageProvider uploads files to Cloudflare R2 (S3-compatible).
type R2StorageProvider struct {
	client    *s3.Client
	bucket    string
	publicURL string
}

func NewR2StorageProvider(cfg *config.Config) (*R2StorageProvider, error) {
	if cfg.R2AccountID == "" {
		return nil, errors.New("R2_ACCOUNT_ID is required for Cloudflare R2 storage driver")
	}
	if cfg.R2AccessKeyID == "" || cfg.R2SecretAccessKey == "" {
		return nil, errors.New("R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY are required for Cloudflare R2 storage driver")
	}
	if cfg.R2BucketName == "" {
		return nil, errors.New("R2_BUCKET_NAME is required for Cloudflare R2 storage driver")
	}
	if cfg.R2PublicURL == "" {
		return nil, errors.New("R2_PUBLIC_URL is required for Cloudflare R2 storage driver (e.g. https://pub-xxx.r2.dev or custom domain)")
	}

	r2Endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID)

	s3Config, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2SecretAccessKey, "")),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for Cloudflare R2: %w", err)
	}

	client := s3.NewFromConfig(s3Config, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(r2Endpoint)
	})

	return &R2StorageProvider{
		client:    client,
		bucket:    cfg.R2BucketName,
		publicURL: cfg.R2PublicURL,
	}, nil
}

func (r *R2StorageProvider) Upload(ctx context.Context, file io.Reader, filename string, contentType string, folder string) (string, error) {
	objectKey := fmt.Sprintf("%s/%s", folder, filename)

	putInput := &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(objectKey),
		Body:        file,
		ContentType: aws.String(contentType),
	}

	_, err := r.client.PutObject(ctx, putInput)
	if err != nil {
		return "", fmt.Errorf("failed to upload object to Cloudflare R2: %w", err)
	}

	// Build public URL
	publicURL := strings.TrimRight(r.publicURL, "/")
	return fmt.Sprintf("%s/%s", publicURL, objectKey), nil
}

// NewStorageProvider creates either a local or Cloudflare R2 storage provider based on configuration.
func NewStorageProvider(cfg *config.Config) (StorageProvider, error) {
	switch strings.ToLower(cfg.StorageDriver) {
	case "r2", "cloudflare_r2", "s3":
		return NewR2StorageProvider(cfg)
	default:
		return NewLocalStorageProvider(cfg.UploadDir, cfg.BaseURL), nil
	}
}
