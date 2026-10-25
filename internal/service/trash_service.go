package service

import (
	"time"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"

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
}

type trashService struct {
	repo repository.TrashRepository
}

func NewTrashService(repo repository.TrashRepository) TrashService {
	return &trashService{repo: repo}
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
