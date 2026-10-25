package service

import (
	"inventory_backend/internal/apperr"
	"strings"
	"time"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"

	"github.com/google/uuid"
)

type BoxService interface {
	Create(userID uuid.UUID, req CreateBoxRequest) (*models.Box, error)
	GetByID(userID uuid.UUID, id uuid.UUID) (*BoxDetailResponse, error)
	List(userID uuid.UUID, parentIDParam string, tree bool) (interface{}, error)
	Update(userID uuid.UUID, id uuid.UUID, req UpdateBoxRequest) (*models.Box, error)
	Move(userID uuid.UUID, id uuid.UUID, newParentID *uuid.UUID) error
	Delete(userID uuid.UUID, id uuid.UUID, cascade bool) error
}

type CreateBoxRequest struct {
	// ID is optional: offline clients pre-generate it so a replayed create is idempotent.
	ID          *uuid.UUID `json:"id"`
	Name        string     `json:"name" binding:"required"`
	Type        string     `json:"type" binding:"required"`
	Description string     `json:"description"`
	ParentID    *uuid.UUID `json:"parentId"`
	Label       string     `json:"label" binding:"required"`
	Icon        *string    `json:"icon"`
}

type UpdateBoxRequest struct {
	// UpdatedAt is the version the client last saw; when set and stale the update is rejected with 409.
	UpdatedAt   *time.Time `json:"updatedAt"`
	Name        *string    `json:"name"`
	Type        *string    `json:"type"`
	Description *string    `json:"description"`
	Label       *string    `json:"label"`
	Icon        *string    `json:"icon"`
}

type BoxDetailResponse struct {
	Box         *models.Box             `json:"box"`
	Breadcrumbs []models.BreadcrumbItem `json:"breadcrumbs"`
	SubBoxes    []models.Box            `json:"subBoxes"`
	// Items holds at most boxDetailItemLimit items; ItemCount is the real total.
	// When ItemsTruncated is true, page through GET /items?boxId=<id> for the rest.
	Items          []models.Item `json:"items"`
	ItemCount      int64         `json:"itemCount"`
	ItemsTruncated bool          `json:"itemsTruncated"`
}

const boxDetailItemLimit = 200

type boxService struct {
	boxRepo  repository.BoxRepository
	itemRepo repository.ItemRepository
}

func NewBoxService(boxRepo repository.BoxRepository, itemRepo repository.ItemRepository) BoxService {
	return &boxService{
		boxRepo:  boxRepo,
		itemRepo: itemRepo,
	}
}

var validBoxTypes = map[string]bool{
	"room": true, "cabinet": true, "shelf": true, "box": true, "drawer": true, "other": true,
}

func (s *boxService) Create(userID uuid.UUID, req CreateBoxRequest) (*models.Box, error) {
	if req.ID != nil {
		if existing, err := s.boxRepo.GetByID(*req.ID, userID); err == nil && existing != nil {
			return existing, nil
		}
	}

	// Verify label uniqueness
	existing, _ := s.boxRepo.GetByLabel(req.Label, &userID)
	if existing != nil {
		return nil, apperr.Conflict("a storage space or box with this label already exists")
	}

	if req.ParentID != nil {
		parentBox, err := s.boxRepo.GetByID(*req.ParentID, userID)
		if err != nil || parentBox == nil {
			return nil, apperr.NotFound("parent box not found or does not belong to you")
		}
	}

	boxType := strings.ToLower(strings.TrimSpace(req.Type))
	if boxType == "" {
		boxType = "box"
	}
	if !validBoxTypes[boxType] {
		return nil, apperr.Invalid("invalid box type, must be one of: room, cabinet, shelf, box, drawer, other")
	}

	box := &models.Box{
		UserID:      userID,
		ParentID:    req.ParentID,
		Name:        req.Name,
		Type:        boxType,
		Description: req.Description,
		Label:       req.Label,
		Icon:        req.Icon,
	}
	if req.ID != nil {
		box.ID = *req.ID
	}

	if err := s.boxRepo.Create(box); err != nil {
		return nil, apperr.ConflictOnUnique(err, "a storage space or box with this label already exists")
	}

	return box, nil
}

func (s *boxService) GetByID(userID uuid.UUID, id uuid.UUID) (*BoxDetailResponse, error) {
	box, err := s.boxRepo.GetByID(id, userID)
	if err != nil {
		return nil, apperr.NotFound("box not found")
	}

	breadcrumbs, err := s.boxRepo.GetBreadcrumbs(id, userID)
	if err != nil {
		breadcrumbs = []models.BreadcrumbItem{}
	}

	subBoxes, err := s.boxRepo.ListByParent(userID, &id)
	if err != nil {
		subBoxes = []models.Box{}
	}
	if err := s.fillCounts(userID, subBoxes); err != nil {
		return nil, err
	}

	items, total, err := s.itemRepo.List(userID, repository.ItemFilter{
		BoxID: &id,
		Limit: boxDetailItemLimit,
	})
	if err != nil {
		return nil, err
	}

	return &BoxDetailResponse{
		Box:         box,
		Breadcrumbs: breadcrumbs,
		SubBoxes:    subBoxes,
		Items:       items,

		ItemCount:      total,
		ItemsTruncated: total > int64(len(items)),
	}, nil
}

func (s *boxService) List(userID uuid.UUID, parentIDParam string, tree bool) (interface{}, error) {
	if tree {
		return s.getBoxTree(userID)
	}

	parentIDParam = strings.TrimSpace(parentIDParam)

	var boxes []models.Box
	var err error

	if parentIDParam == "" {
		// When parentId is not specified, return all boxes belonging to this user
		boxes, err = s.boxRepo.ListAll(userID)
	} else if strings.ToLower(parentIDParam) == "root" {
		// When parentId is "root", return top-level boxes only (parent_id IS NULL)
		boxes, err = s.boxRepo.ListByParent(userID, nil)
	} else {
		parsed, parseErr := uuid.Parse(parentIDParam)
		if parseErr != nil {
			return nil, apperr.Invalid("invalid parentId format")
		}
		boxes, err = s.boxRepo.ListByParent(userID, &parsed)
	}

	if err != nil {
		return nil, err
	}

	if err := s.fillCounts(userID, boxes); err != nil {
		return nil, err
	}

	return boxes, nil
}

func (s *boxService) getBoxTree(userID uuid.UUID) ([]*models.Box, error) {
	allBoxes, err := s.boxRepo.ListAll(userID)
	if err != nil {
		return nil, err
	}

	if err := s.fillCounts(userID, allBoxes); err != nil {
		return nil, err
	}

	boxMap := make(map[uuid.UUID]*models.Box)
	for i := range allBoxes {
		b := allBoxes[i]
		b.Children = make([]*models.Box, 0)
		boxMap[b.ID] = &b
	}

	var rootBoxes []*models.Box
	for _, b := range boxMap {
		if b.ParentID == nil {
			rootBoxes = append(rootBoxes, b)
		} else if parent, ok := boxMap[*b.ParentID]; ok {
			parent.Children = append(parent.Children, b)
		}
	}

	if rootBoxes == nil {
		rootBoxes = make([]*models.Box, 0)
	}
	return rootBoxes, nil
}

func (s *boxService) Update(userID uuid.UUID, id uuid.UUID, req UpdateBoxRequest) (*models.Box, error) {
	box, err := s.boxRepo.GetByID(id, userID)
	if err != nil {
		return nil, apperr.NotFound("box not found")
	}
	if isStale(req.UpdatedAt, box.UpdatedAt) {
		return nil, apperr.Conflict("box was modified since you last loaded it, please reload and try again")
	}

	if req.Name != nil {
		box.Name = *req.Name
	}
	if req.Type != nil {
		boxType := strings.ToLower(strings.TrimSpace(*req.Type))
		if boxType != "" {
			if !validBoxTypes[boxType] {
				return nil, apperr.Invalid("invalid box type, must be one of: room, cabinet, shelf, box, drawer, other")
			}
			box.Type = boxType
		}
	}
	if req.Description != nil {
		box.Description = *req.Description
	}
	if req.Icon != nil {
		box.Icon = req.Icon
	}
	if req.Label != nil && *req.Label != box.Label {
		existing, _ := s.boxRepo.GetByLabel(*req.Label, &userID)
		if existing != nil && existing.ID != box.ID {
			return nil, apperr.Conflict("label is already taken by another box")
		}
		box.Label = *req.Label
	}

	if err := s.boxRepo.Update(box); err != nil {
		return nil, apperr.ConflictOnUnique(err, "label is already taken by another box")
	}

	return box, nil
}

func (s *boxService) Move(userID uuid.UUID, id uuid.UUID, newParentID *uuid.UUID) error {
	box, err := s.boxRepo.GetByID(id, userID)
	if err != nil {
		return apperr.NotFound("box not found")
	}

	if newParentID != nil {
		// Business Rule: Cycle Prevention
		if *newParentID == box.ID {
			return apperr.Invalid("cannot move a box inside itself")
		}

		// Ensure new parent exists
		_, err := s.boxRepo.GetByID(*newParentID, userID)
		if err != nil {
			return apperr.NotFound("target parent box not found")
		}

		// Check if new parent is a descendant of current box
		isDesc, err := s.boxRepo.IsDescendant(box.ID, *newParentID, userID)
		if err != nil {
			return err
		}
		if isDesc {
			return apperr.Invalid("cannot move a box inside one of its own sub-boxes (cycle detected)")
		}
	}

	// Re-checked under a per-user lock so concurrent moves cannot form a cycle.
	return s.boxRepo.Move(box.ID, userID, newParentID)
}

func (s *boxService) Delete(userID uuid.UUID, id uuid.UUID, cascade bool) error {
	return s.boxRepo.Delete(id, userID, cascade)
}

// fillCounts sets ItemCount/SubBoxCount on every box using two grouped queries
// (instead of two COUNT queries per box).
func (s *boxService) fillCounts(userID uuid.UUID, boxes []models.Box) error {
	if len(boxes) == 0 {
		return nil
	}
	items, subBoxes, err := s.boxRepo.CountsByBox(userID)
	if err != nil {
		return err
	}
	for i := range boxes {
		boxes[i].ItemCount = int(items[boxes[i].ID])
		boxes[i].SubBoxCount = int(subBoxes[boxes[i].ID])
	}
	return nil
}
