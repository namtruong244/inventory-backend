package service

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/storage"

	"github.com/google/uuid"
)

// MaxUploadSize is the largest accepted upload (bytes).
const MaxUploadSize = 10 << 20

type MediaService interface {
	Upload(userID uuid.UUID, file *multipart.FileHeader, folder string) (*MediaUploadResponse, error)
	UploadWithContext(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader, folder string) (*MediaUploadResponse, error)
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

func (s *mediaService) Upload(userID uuid.UUID, file *multipart.FileHeader, folder string) (*MediaUploadResponse, error) {
	return s.UploadWithContext(context.Background(), userID, file, folder)
}

// UploadWithContext stores the file under the uploader's own folder, so the URL
// proves ownership (see storage.OwnerOf). Validation failures are apperr.Invalid
// (safe to show); anything else is an internal error the handler must not echo.
func (s *mediaService) UploadWithContext(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader, folder string) (*MediaUploadResponse, error) {
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
		return nil, apperr.Invalid("unsupported file extension (allowed: jpg, jpeg, png, webp, gif, heic, pdf)")
	}

	if file.Size > MaxUploadSize {
		return nil, apperr.Invalid(fmt.Sprintf("file is too large (max %d MB)", MaxUploadSize>>20))
	}

	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	// Trust the file's actual bytes, not the client-supplied Content-Type or extension.
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, fmt.Errorf("failed to read uploaded file: %w", err)
	}
	head = head[:n]
	if !contentMatchesExtension(head, ext) {
		return nil, apperr.Invalid("file content does not match its extension")
	}
	contentType := defaultMime
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to read uploaded file: %w", err)
	}

	uniqueFilename := uuid.New().String() + ext

	fileURL, err := s.storageProvider.Upload(ctx, src, uniqueFilename, contentType, storage.UserFolder(folder, userID))
	if err != nil {
		return nil, fmt.Errorf("failed to upload media: %w", err)
	}

	return &MediaUploadResponse{
		URL: fileURL,
	}, nil
}

// contentMatchesExtension sniffs the leading bytes and checks they fit the extension.
func contentMatchesExtension(head []byte, ext string) bool {
	if ext == ".heic" {
		// ISO base media file: bytes 4-8 are "ftyp", followed by a HEIF brand
		if len(head) < 12 || string(head[4:8]) != "ftyp" {
			return false
		}
		switch string(head[8:12]) {
		case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
			return true
		}
		return false
	}

	detected := http.DetectContentType(head)
	switch ext {
	case ".jpg", ".jpeg":
		return detected == "image/jpeg"
	case ".png":
		return detected == "image/png"
	case ".webp":
		return detected == "image/webp"
	case ".gif":
		return detected == "image/gif"
	case ".pdf":
		return detected == "application/pdf"
	}
	return false
}
