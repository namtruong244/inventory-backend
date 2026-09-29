package service

import (
	"errors"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"

	"github.com/google/uuid"
)

type ItemService interface {
	Create(userID uuid.UUID, req CreateItemRequest) (*models.Item, error)
	GetByID(userID uuid.UUID, id uuid.UUID) (*ItemDetailResponse, error)
	List(userID uuid.UUID, filter repository.ItemFilter) ([]models.Item, int64, error)
	Update(userID uuid.UUID, id uuid.UUID, req UpdateItemRequest) (*models.Item, error)
	Move(userID uuid.UUID, id uuid.UUID, targetBoxID uuid.UUID) error
	UpdateQuantity(userID uuid.UUID, id uuid.UUID, quantity float64) error
	Delete(userID uuid.UUID, id uuid.UUID) error
}

type CreateItemRequest struct {
	Name               string                 `json:"name" binding:"required"`
	Description        string                 `json:"description"`
	BoxID              uuid.UUID              `json:"boxId" binding:"required"`
	CategoryID         string                 `json:"categoryId" binding:"required"`
	Quantity           *float64               `json:"quantity"`
	Unit               *string                `json:"unit"`
	MinQuantity        *float64               `json:"minQuantity"`
	Status             *string                `json:"status"`
	PurchasePrice      *float64               `json:"purchasePrice"`
	PurchaseDate       *string                `json:"purchaseDate"`
	SerialNumber       *string                `json:"serialNumber"`
	Barcode            *string                `json:"barcode"`
	WarrantyExpiryDate *string                `json:"warrantyExpiryDate"`
	ExpiryDate         *string                `json:"expiryDate"`
	Photos             []string               `json:"photos"`
	ReceiptPhotos      []string               `json:"receiptPhotos"`
	CustomAttributes   map[string]interface{} `json:"customAttributes"`
}

type UpdateItemRequest struct {
	Name               *string                `json:"name"`
	Description        *string                `json:"description"`
	BoxID              *uuid.UUID             `json:"boxId"`
	CategoryID         *string                `json:"categoryId"`
	Quantity           *float64               `json:"quantity"`
	Unit               *string                `json:"unit"`
	MinQuantity        *float64               `json:"minQuantity"`
	Status             *string                `json:"status"`
	PurchasePrice      *float64               `json:"purchasePrice"`
	PurchaseDate       *string                `json:"purchaseDate"`
	SerialNumber       *string                `json:"serialNumber"`
	Barcode            *string                `json:"barcode"`
	WarrantyExpiryDate *string                `json:"warrantyExpiryDate"`
	ExpiryDate         *string                `json:"expiryDate"`
	Photos             []string               `json:"photos"`
	ReceiptPhotos      []string               `json:"receiptPhotos"`
	CustomAttributes   map[string]interface{} `json:"customAttributes"`
}

type ItemDetailResponse struct {
	*models.Item
	Breadcrumbs    []models.BreadcrumbItem `json:"breadcrumbs"`
	CurrentLoan    *models.LendingRecord   `json:"currentLoan,omitempty"`
	LendingHistory []models.LendingRecord  `json:"lendingHistory"`
}

type itemService struct {
	itemRepo     repository.ItemRepository
	boxRepo      repository.BoxRepository
	lendingRepo  repository.LendingRepository
	settingsRepo repository.SettingsRepository
}

func NewItemService(
	itemRepo repository.ItemRepository,
	boxRepo repository.BoxRepository,
	lendingRepo repository.LendingRepository,
	settingsRepo repository.SettingsRepository,
) ItemService {
	return &itemService{
		itemRepo:     itemRepo,
		boxRepo:      boxRepo,
		lendingRepo:  lendingRepo,
		settingsRepo: settingsRepo,
	}
}

func (s *itemService) Create(userID uuid.UUID, req CreateItemRequest) (*models.Item, error) {
	// Verify target box exists
	box, err := s.boxRepo.GetByID(req.BoxID, userID)
	if err != nil || box == nil {
		return nil, errors.New("target box does not exist or does not belong to you")
	}

	quantity := 1.0
	if req.Quantity != nil {
		quantity = *req.Quantity
	}

	unit := "pcs"
	if req.Unit != nil && *req.Unit != "" {
		unit = *req.Unit
	} else if settings, err := s.settingsRepo.GetByUserID(userID); err == nil && settings != nil {
		if settings.DefaultUnit != "" {
			unit = settings.DefaultUnit
		}
	}

	status := "stored"
	if req.Status != nil && *req.Status != "" {
		status = *req.Status
	}

	item := &models.Item{
		UserID:             userID,
		BoxID:              req.BoxID,
		CategoryID:         req.CategoryID,
		Name:               req.Name,
		Description:        req.Description,
		Quantity:           quantity,
		Unit:               unit,
		MinQuantity:        req.MinQuantity,
		Status:             status,
		PurchasePrice:      req.PurchasePrice,
		PurchaseDate:       req.PurchaseDate,
		SerialNumber:       req.SerialNumber,
		Barcode:            req.Barcode,
		WarrantyExpiryDate: req.WarrantyExpiryDate,
		ExpiryDate:         req.ExpiryDate,
		Photos:             req.Photos,
		ReceiptPhotos:      req.ReceiptPhotos,
		CustomAttributes:   req.CustomAttributes,
	}

	if err := s.itemRepo.Create(item); err != nil {
		return nil, err
	}

	item.BoxName = box.Name
	item.IsLowStock = item.ComputeIsLowStock()
	return item, nil
}

func (s *itemService) GetByID(userID uuid.UUID, id uuid.UUID) (*ItemDetailResponse, error) {
	item, err := s.itemRepo.GetByID(id, userID)
	if err != nil {
		return nil, errors.New("item not found")
	}

	item.IsLowStock = item.ComputeIsLowStock()

	// Breadcrumbs of the item's box
	breadcrumbs, err := s.boxRepo.GetBreadcrumbs(item.BoxID, userID)
	if err != nil {
		breadcrumbs = []models.BreadcrumbItem{}
	}

	// Current active loan if item is lent
	var currentLoan *models.LendingRecord
	if item.Status == "lent" {
		loan, _ := s.lendingRepo.GetActiveLoanByItemID(item.ID)
		currentLoan = loan
	}

	// Loan history
	history, err := s.lendingRepo.ListHistoryByItemID(userID, item.ID)
	if err != nil {
		history = []models.LendingRecord{}
	}

	return &ItemDetailResponse{
		Item:           item,
		Breadcrumbs:    breadcrumbs,
		CurrentLoan:    currentLoan,
		LendingHistory: history,
	}, nil
}

func (s *itemService) List(userID uuid.UUID, filter repository.ItemFilter) ([]models.Item, int64, error) {
	// Look up expiry warning days from settings
	if filter.IsExpiring != nil && *filter.IsExpiring && filter.ExpiryWarningDays == 0 {
		settings, err := s.settingsRepo.GetByUserID(userID)
		if err == nil && settings != nil && settings.ExpiryWarningDays > 0 {
			filter.ExpiryWarningDays = settings.ExpiryWarningDays
		} else {
			filter.ExpiryWarningDays = 30
		}
	}

	items, total, err := s.itemRepo.List(userID, filter)
	if err != nil {
		return nil, 0, err
	}

	for i := range items {
		items[i].IsLowStock = items[i].ComputeIsLowStock()
	}

	return items, total, nil
}

func (s *itemService) Update(userID uuid.UUID, id uuid.UUID, req UpdateItemRequest) (*models.Item, error) {
	item, err := s.itemRepo.GetByID(id, userID)
	if err != nil {
		return nil, errors.New("item not found")
	}

	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.Description != nil {
		item.Description = *req.Description
	}
	if req.BoxID != nil && *req.BoxID != item.BoxID {
		box, err := s.boxRepo.GetByID(*req.BoxID, userID)
		if err != nil || box == nil {
			return nil, errors.New("target box does not exist or does not belong to you")
		}
		item.BoxID = *req.BoxID
	}
	if req.CategoryID != nil {
		item.CategoryID = *req.CategoryID
	}
	if req.Quantity != nil {
		item.Quantity = *req.Quantity
	}
	if req.Unit != nil {
		item.Unit = *req.Unit
	}
	if req.MinQuantity != nil {
		item.MinQuantity = req.MinQuantity
	}
	if req.Status != nil {
		item.Status = *req.Status
	}
	if req.PurchasePrice != nil {
		item.PurchasePrice = req.PurchasePrice
	}
	if req.PurchaseDate != nil {
		item.PurchaseDate = req.PurchaseDate
	}
	if req.SerialNumber != nil {
		item.SerialNumber = req.SerialNumber
	}
	if req.Barcode != nil {
		item.Barcode = req.Barcode
	}
	if req.WarrantyExpiryDate != nil {
		item.WarrantyExpiryDate = req.WarrantyExpiryDate
	}
	if req.ExpiryDate != nil {
		item.ExpiryDate = req.ExpiryDate
	}
	if req.Photos != nil {
		item.Photos = req.Photos
	}
	if req.ReceiptPhotos != nil {
		item.ReceiptPhotos = req.ReceiptPhotos
	}
	if req.CustomAttributes != nil {
		item.CustomAttributes = req.CustomAttributes
	}

	if err := s.itemRepo.Update(item); err != nil {
		return nil, err
	}

	item.IsLowStock = item.ComputeIsLowStock()
	return item, nil
}

func (s *itemService) Move(userID uuid.UUID, id uuid.UUID, targetBoxID uuid.UUID) error {
	box, err := s.boxRepo.GetByID(targetBoxID, userID)
	if err != nil || box == nil {
		return errors.New("target box does not exist or does not belong to you")
	}

	return s.itemRepo.Move(id, userID, targetBoxID)
}

func (s *itemService) UpdateQuantity(userID uuid.UUID, id uuid.UUID, quantity float64) error {
	if quantity < 0 {
		return errors.New("quantity cannot be negative")
	}
	return s.itemRepo.UpdateQuantity(id, userID, quantity)
}

func (s *itemService) Delete(userID uuid.UUID, id uuid.UUID) error {
	return s.itemRepo.Delete(id, userID)
}
