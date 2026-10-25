package service

import (
	"inventory_backend/internal/apperr"
	"strings"

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
	Name        string     `json:"name" binding:"required"`
	Type        string     `json:"type" binding:"required"`
	Description string     `json:"description"`
	ParentID    *uuid.UUID `json:"parentId"`
	Label       string     `json:"label" binding:"required"`
	Icon        *string    `json:"icon"`
}

type UpdateBoxRequest struct {
	Name        *string `json:"name"`
	Type        *string `json:"type"`
	Description *string `json:"description"`
	Label       *string `json:"label"`
	Icon        *string `json:"icon"`
}

type BoxDetailResponse struct {
	Box         *models.Box             `json:"box"`
	Breadcrumbs []models.BreadcrumbItem `json:"breadcrumbs"`
	SubBoxes    []models.Box            `json:"subBoxes"`
	Items       []models.Item           `json:"items"`
}

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
	for i := range subBoxes {
		subBoxes[i].ItemCount, _ = s.getBoxItemCount(subBoxes[i].ID, userID)
		subBoxes[i].SubBoxCount, _ = s.getBoxSubBoxCount(subBoxes[i].ID, userID)
	}

	items, _, err := s.itemRepo.List(userID, repository.ItemFilter{
		BoxID: &id,
		Limit: 1000,
	})
	if err != nil {
		items = []models.Item{}
	}

	return &BoxDetailResponse{
		Box:         box,
		Breadcrumbs: breadcrumbs,
		SubBoxes:    subBoxes,
		Items:       items,
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

	for i := range boxes {
		boxes[i].ItemCount, _ = s.getBoxItemCount(boxes[i].ID, userID)
		boxes[i].SubBoxCount, _ = s.getBoxSubBoxCount(boxes[i].ID, userID)
	}

	return boxes, nil
}

func (s *boxService) getBoxTree(userID uuid.UUID) ([]*models.Box, error) {
	allBoxes, err := s.boxRepo.ListAll(userID)
	if err != nil {
		return nil, err
	}

	boxMap := make(map[uuid.UUID]*models.Box)
	for i := range allBoxes {
		b := allBoxes[i]
		b.ItemCount, _ = s.getBoxItemCount(b.ID, userID)
		b.SubBoxCount, _ = s.getBoxSubBoxCount(b.ID, userID)
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

	box.ParentID = newParentID
	return s.boxRepo.Update(box)
}

func (s *boxService) Delete(userID uuid.UUID, id uuid.UUID, cascade bool) error {
	return s.boxRepo.Delete(id, userID, cascade)
}

func (s *boxService) getBoxItemCount(boxID, userID uuid.UUID) (int, error) {
	c, err := s.boxRepo.CountItemsInBox(boxID, userID)
	return int(c), err
}

func (s *boxService) getBoxSubBoxCount(boxID, userID uuid.UUID) (int, error) {
	c, err := s.boxRepo.CountSubBoxesInBox(boxID, userID)
	return int(c), err
}
