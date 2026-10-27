package service

import (
	"strings"

	"inventory_backend/internal/apperr"

	"inventory_backend/internal/models"
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
	// Scanners and pasted text often add a trailing newline or space.
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, apperr.Invalid("code parameter is required")
	}

	boxResult := func(box *models.Box) *ScanLookupResult {
		itemCount, _ := s.boxRepo.CountItemsInBox(box.ID, userID)
		return &ScanLookupResult{
			Type: "box",
			Payload: ScanBoxPayload{
				ID:        box.ID,
				Name:      box.Name,
				Type:      box.Type,
				ItemCount: int(itemCount),
			},
		}
	}

	// 1. Check boxes by label
	box, err := s.boxRepo.GetByLabel(code, &userID)
	if err == nil && box != nil {
		return boxResult(box), nil
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

	// 3. A box QR code can carry the box id instead of its label
	if boxID, perr := uuid.Parse(code); perr == nil {
		if box, err := s.boxRepo.GetByID(boxID, userID); err == nil && box != nil {
			return boxResult(box), nil
		}
	}

	return nil, apperr.NotFound("no matching box or item found for the provided code")
}
