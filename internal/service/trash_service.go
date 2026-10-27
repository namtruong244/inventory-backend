package service

import (
	"context"
	"log"
	"time"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/storage"

	"github.com/google/uuid"
)

type TrashContents struct {
	Items []models.Item `json:"items"`
	Boxes []models.Box  `json:"boxes"`
}

type TrashService interface {
	List(userID uuid.UUID) (*TrashContents, error)
	RestoreItem(userID, id uuid.UUID) error
	RestoreBox(userID, id uuid.UUID) error
	PurgeItem(userID, id uuid.UUID) error
	PurgeBox(userID, id uuid.UUID) error
	// Empty purges everything, or only what was trashed more than olderThanDays ago when > 0.
	Empty(userID uuid.UUID, olderThanDays int) (int64, error)
	// PurgeExpired purges, for every user, what has been in the trash longer than
	// olderThanDays, and removes the media files of the purged items.
	PurgeExpired(olderThanDays int) (int64, error)
}

type trashService struct {
	repo  repository.TrashRepository
	files storage.StorageProvider
}

// TrashOption customises optional dependencies of the trash service.
type TrashOption func(*trashService)

// WithTrashFileStorage lets PurgeExpired remove the purged items' media too.
func WithTrashFileStorage(p storage.StorageProvider) TrashOption {
	return func(s *trashService) { s.files = p }
}

func NewTrashService(repo repository.TrashRepository, opts ...TrashOption) TrashService {
	s := &trashService{repo: repo}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *trashService) List(userID uuid.UUID) (*TrashContents, error) {
	items, boxes, err := s.repo.List(userID)
	if err != nil {
		return nil, err
	}
	return &TrashContents{Items: items, Boxes: boxes}, nil
}

func (s *trashService) RestoreItem(userID, id uuid.UUID) error { return s.repo.RestoreItem(id, userID) }
func (s *trashService) RestoreBox(userID, id uuid.UUID) error  { return s.repo.RestoreBox(id, userID) }
func (s *trashService) PurgeItem(userID, id uuid.UUID) error   { return s.repo.PurgeItem(id, userID) }
func (s *trashService) PurgeBox(userID, id uuid.UUID) error    { return s.repo.PurgeBox(id, userID) }

func (s *trashService) Empty(userID uuid.UUID, olderThanDays int) (int64, error) {
	if olderThanDays < 0 {
		return 0, apperr.Invalid("olderThanDays must be >= 0")
	}
	var cutoff time.Time
	if olderThanDays > 0 {
		cutoff = time.Now().UTC().AddDate(0, 0, -olderThanDays)
	}
	return s.repo.Empty(userID, cutoff)
}

func (s *trashService) PurgeExpired(olderThanDays int) (int64, error) {
	if olderThanDays <= 0 {
		return 0, apperr.Invalid("olderThanDays must be > 0")
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -olderThanDays)
	users, err := s.repo.UsersWithTrashBefore(cutoff)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, uid := range users {
		// A failure for one user must not stop the others.
		var files []string
		if s.files != nil {
			if files, err = s.repo.MediaTrashedBefore(uid, cutoff); err != nil {
				log.Printf("trash purge: listing media for %s failed: %v", uid, err)
				files = nil
			}
		}
		n, err := s.repo.Empty(uid, cutoff)
		if err != nil {
			log.Printf("trash purge: user %s failed: %v", uid, err)
			continue
		}
		total += n
		for _, f := range files {
			if storage.BelongsToOther(f, uid) {
				log.Printf("trash purge: skipping %s, it belongs to another user", f)
				continue
			}
			if err := s.files.Delete(context.Background(), f); err != nil {
				log.Printf("trash purge: could not remove %s: %v", f, err)
			}
		}
	}
	return total, nil
}
