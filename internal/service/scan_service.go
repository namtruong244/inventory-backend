package service

import (
	"errors"

	"inventory_backend/internal/repository"

	"github.com/google/uuid"
)

type ScanService interface {
	Lookup(userID uuid.UUID, code string) (*ScanLookupResult, error)
}

type ScanLookupResult struct {
	Type    string      `json:"type"` // "box" or "item"
	Payload interface{} `json:"payload"`
}

type ScanBoxPayload struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	ItemCount int       `json:"itemCount"`
}

type ScanItemPayload struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	BoxID      uuid.UUID `json:"boxId"`
	Quantity   float64   `json:"quantity"`
	Status     string    `json:"status"`
	Barcode    *string   `json:"barcode,omitempty"`
	IsLowStock bool      `json:"isLowStock"`
}

type scanService struct {
	boxRepo  repository.BoxRepository
	itemRepo repository.ItemRepository
}

func NewScanService(boxRepo repository.BoxRepository, itemRepo repository.ItemRepository) ScanService {
	return &scanService{
		boxRepo:  boxRepo,
		itemRepo: itemRepo,
	}
}

func (s *scanService) Lookup(userID uuid.UUID, code string) (*ScanLookupResult, error) {
	if code == "" {
		return nil, errors.New("code parameter is required")
	}

	// 1. Check boxes by label
	box, err := s.boxRepo.GetByLabel(code, &userID)
	if err == nil && box != nil {
		itemCount, _ := s.boxRepo.CountItemsInBox(box.ID)
		return &ScanLookupResult{
			Type: "box",
			Payload: ScanBoxPayload{
				ID:        box.ID,
				Name:      box.Name,
				Type:      box.Type,
				ItemCount: int(itemCount),
			},
		}, nil
	}

	// 2. Check items by barcode, serial_number, or ID
	item, err := s.itemRepo.GetByCode(code, &userID)
	if err == nil && item != nil {
		item.IsLowStock = item.ComputeIsLowStock()
		return &ScanLookupResult{
			Type: "item",
			Payload: ScanItemPayload{
				ID:         item.ID,
				Name:       item.Name,
				BoxID:      item.BoxID,
				Quantity:   item.Quantity,
				Status:     item.Status,
				Barcode:    item.Barcode,
				IsLowStock: item.IsLowStock,
			},
		}, nil
	}

	return nil, errors.New("no matching box or item found for the provided code")
}
