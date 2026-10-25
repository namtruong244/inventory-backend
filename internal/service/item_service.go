package service

import (
	"log"
	"strings"
	"time"

	"inventory_backend/internal/apperr"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/utils"

	"github.com/google/uuid"
)

var validItemStatuses = map[string]bool{
	"stored": true, "inUse": true, "lent": true, "broken": true, "disposed": true, "lost": true,
}

// normalizeDate accepts "YYYY-MM-DD" or an RFC3339 timestamp and stores only
// the calendar date as written, so string comparison against dates is correct.
func normalizeDate(field string, v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(*v)
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			d := t.Format("2006-01-02")
			return &d, nil
		}
	}
	return nil, apperr.Invalid(field + " must be a date in YYYY-MM-DD format")
}

type ItemService interface {
	Create(userID uuid.UUID, req CreateItemRequest) (*models.Item, error)
	GetByID(userID uuid.UUID, id uuid.UUID) (*ItemDetailResponse, error)
	List(userID uuid.UUID, filter repository.ItemFilter) ([]models.Item, int64, error)
	Update(userID uuid.UUID, id uuid.UUID, req UpdateItemRequest) (*models.Item, error)
	Move(userID uuid.UUID, id uuid.UUID, targetBoxID uuid.UUID) error
	UpdateQuantity(userID uuid.UUID, id uuid.UUID, quantity float64, reason, note string) error
	Movements(userID uuid.UUID, id uuid.UUID) (*MovementHistory, error)
	Delete(userID uuid.UUID, id uuid.UUID) error
}

type CreateItemRequest struct {
	// ID is optional: offline clients pre-generate it so a replayed create is idempotent.
	ID                 *uuid.UUID             `json:"id"`
	Name               string                 `json:"name" binding:"required"`
	Description        string                 `json:"description"`
	BoxID              uuid.UUID              `json:"boxId" binding:"required"`
	CategoryID         string                 `json:"categoryId" binding:"required"`
	Quantity           *float64               `json:"quantity"`
	Unit               *string                `json:"unit"`
	MinQuantity        *float64               `json:"minQuantity"`
	IsConsumable       *bool                  `json:"isConsumable"`
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
	ClearMinQuantity   bool                   `json:"clearMinQuantity"` // true removes the stock threshold
	IsConsumable       *bool                  `json:"isConsumable"`
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
	categoryRepo repository.CategoryRepository
	movementRepo repository.MovementRepository
}

func NewItemService(
	itemRepo repository.ItemRepository,
	boxRepo repository.BoxRepository,
	lendingRepo repository.LendingRepository,
	settingsRepo repository.SettingsRepository,
	categoryRepo repository.CategoryRepository,
	movementRepo repository.MovementRepository,
) ItemService {
	return &itemService{
		itemRepo:     itemRepo,
		boxRepo:      boxRepo,
		lendingRepo:  lendingRepo,
		settingsRepo: settingsRepo,
		categoryRepo: categoryRepo,
		movementRepo: movementRepo,
	}
}

// resolveCategory checks that the category exists for this user and returns the
// canonical value stored on items (the category name, which is what clients send).
func (s *itemService) resolveCategory(userID uuid.UUID, value string) (string, error) {
	cat, err := s.categoryRepo.Resolve(userID, strings.TrimSpace(value))
	if err != nil || cat == nil {
		return "", apperr.Invalid("category does not exist")
	}
	return cat.Name, nil
}

func (s *itemService) Create(userID uuid.UUID, req CreateItemRequest) (*models.Item, error) {
	if req.ID != nil {
		if existing, err := s.itemRepo.GetByID(*req.ID, userID); err == nil && existing != nil {
			return existing, nil
		}
	}

	// Verify target box exists
	box, err := s.boxRepo.GetByID(req.BoxID, userID)
	if err != nil || box == nil {
		return nil, apperr.NotFound("target box does not exist or does not belong to you")
	}

	categoryName, err := s.resolveCategory(userID, req.CategoryID)
	if err != nil {
		return nil, err
	}

	quantity := 1.0
	if req.Quantity != nil {
		quantity = *req.Quantity
	}
	if quantity < 0 {
		return nil, apperr.Invalid("quantity cannot be negative")
	}
	if req.MinQuantity != nil && *req.MinQuantity < 0 {
		return nil, apperr.Invalid("minQuantity cannot be negative")
	}
	purchaseDate, err := normalizeDate("purchaseDate", req.PurchaseDate)
	if err != nil {
		return nil, err
	}
	warrantyDate, err := normalizeDate("warrantyExpiryDate", req.WarrantyExpiryDate)
	if err != nil {
		return nil, err
	}
	expiryDate, err := normalizeDate("expiryDate", req.ExpiryDate)
	if err != nil {
		return nil, err
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
	if !validItemStatuses[status] {
		return nil, apperr.Invalid("invalid status, must be one of: stored, inUse, broken, disposed, lost")
	}
	if status == "lent" {
		return nil, apperr.Invalid("status 'lent' can only be set by lending the item")
	}

	isConsumable := false
	if req.IsConsumable != nil {
		isConsumable = *req.IsConsumable
	}

	item := &models.Item{
		UserID:             userID,
		BoxID:              req.BoxID,
		CategoryID:         categoryName,
		Name:               req.Name,
		Description:        req.Description,
		Quantity:           quantity,
		Unit:               unit,
		MinQuantity:        req.MinQuantity,
		IsConsumable:       isConsumable,
		Status:             status,
		PurchasePrice:      req.PurchasePrice,
		PurchaseDate:       purchaseDate,
		SerialNumber:       req.SerialNumber,
		Barcode:            req.Barcode,
		WarrantyExpiryDate: warrantyDate,
		ExpiryDate:         expiryDate,
		Photos:             req.Photos,
		ReceiptPhotos:      req.ReceiptPhotos,
		CustomAttributes:   req.CustomAttributes,
	}

	if req.ID != nil {
		item.ID = *req.ID
	}

	if err := s.itemRepo.Create(item); err != nil {
		return nil, apperr.ConflictOnUnique(err, "an item with this id already exists")
	}
	s.recordMovement(item, item.Quantity, models.MovementInitial, "")

	item.BoxName = box.Name
	item.IsLowStock = item.ComputeIsLowStock()
	return item, nil
}

func (s *itemService) GetByID(userID uuid.UUID, id uuid.UUID) (*ItemDetailResponse, error) {
	item, err := s.itemRepo.GetByID(id, userID)
	if err != nil {
		return nil, apperr.NotFoundOr(err, "item not found")
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
	// Settings decide the user's "today" and the expiring-soon window
	settings, err := s.settingsRepo.GetByUserID(userID)
	tz := "UTC"
	if err == nil && settings != nil {
		tz = settings.Timezone
	}
	filter.Today = utils.TodayIn(tz)
	if filter.IsExpiring != nil && *filter.IsExpiring && filter.ExpiryWarningDays == 0 {
		filter.ExpiryWarningDays = 30
		if err == nil && settings != nil && settings.ExpiryWarningDays > 0 {
			filter.ExpiryWarningDays = settings.ExpiryWarningDays
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
		return nil, apperr.NotFoundOr(err, "item not found")
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
			return nil, apperr.NotFound("target box does not exist or does not belong to you")
		}
		item.BoxID = *req.BoxID
	}
	if req.CategoryID != nil {
		categoryName, err := s.resolveCategory(userID, *req.CategoryID)
		if err != nil {
			return nil, err
		}
		item.CategoryID = categoryName
	}
	oldQuantity := item.Quantity
	if req.Quantity != nil {
		if *req.Quantity < 0 {
			return nil, apperr.Invalid("quantity cannot be negative")
		}
		item.Quantity = *req.Quantity
	}
	if req.Unit != nil {
		item.Unit = *req.Unit
	}
	if req.ClearMinQuantity {
		item.MinQuantity = nil
	} else if req.MinQuantity != nil {
		if *req.MinQuantity < 0 {
			return nil, apperr.Invalid("minQuantity cannot be negative")
		}
		item.MinQuantity = req.MinQuantity
	}
	if req.IsConsumable != nil {
		item.IsConsumable = *req.IsConsumable
	}
	if req.Status != nil && *req.Status != item.Status {
		if !validItemStatuses[*req.Status] {
			return nil, apperr.Invalid("invalid status, must be one of: stored, inUse, broken, disposed, lost")
		}
		// 'lent' is owned by the lending flow so status and open loans never disagree.
		if *req.Status == "lent" || item.Status == "lent" {
			return nil, apperr.Invalid("status 'lent' can only be changed through the lending API")
		}
		item.Status = *req.Status
	}
	if req.PurchasePrice != nil {
		item.PurchasePrice = req.PurchasePrice
	}
	if req.PurchaseDate != nil {
		d, err := normalizeDate("purchaseDate", req.PurchaseDate)
		if err != nil {
			return nil, err
		}
		item.PurchaseDate = d
	}
	if req.SerialNumber != nil {
		item.SerialNumber = req.SerialNumber
	}
	if req.Barcode != nil {
		item.Barcode = req.Barcode
	}
	if req.WarrantyExpiryDate != nil {
		d, err := normalizeDate("warrantyExpiryDate", req.WarrantyExpiryDate)
		if err != nil {
			return nil, err
		}
		item.WarrantyExpiryDate = d
	}
	if req.ExpiryDate != nil {
		d, err := normalizeDate("expiryDate", req.ExpiryDate)
		if err != nil {
			return nil, err
		}
		item.ExpiryDate = d
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
		return nil, apperr.ConflictOnUnique(err, "item conflicts with an existing item")
	}

	if item.Quantity != oldQuantity {
		s.recordMovement(item, item.Quantity-oldQuantity, models.MovementAdjust, "")
	}

	item.IsLowStock = item.ComputeIsLowStock()
	return item, nil
}

func (s *itemService) Move(userID uuid.UUID, id uuid.UUID, targetBoxID uuid.UUID) error {
	box, err := s.boxRepo.GetByID(targetBoxID, userID)
	if err != nil || box == nil {
		return apperr.NotFound("target box does not exist or does not belong to you")
	}

	if err := s.itemRepo.Move(id, userID, targetBoxID); err != nil {
		return err
	}
	if item, err := s.itemRepo.GetByID(id, userID); err == nil && item != nil {
		s.recordMovement(item, 0, models.MovementMove, "box "+targetBoxID.String())
	}
	return nil
}

func (s *itemService) UpdateQuantity(userID uuid.UUID, id uuid.UUID, quantity float64, reason, note string) error {
	if quantity < 0 {
		return apperr.Invalid("quantity cannot be negative")
	}
	if reason == "" {
		reason = models.MovementAdjust
	}
	if !models.ValidMovementReasons[reason] {
		return apperr.Invalid("reason must be one of: restock, consume, damaged, lost, correction, adjust")
	}
	item, err := s.itemRepo.GetByID(id, userID)
	if err != nil || item == nil {
		return apperr.NotFound("item not found")
	}
	old := item.Quantity
	if err := s.itemRepo.UpdateQuantity(id, userID, quantity); err != nil {
		return err
	}
	item.Quantity = quantity
	if quantity != old {
		s.recordMovement(item, quantity-old, reason, note)
	}
	return nil
}

// MovementHistory is the audit trail of an item plus a consumption estimate.
type MovementHistory struct {
	Movements []models.StockMovement `json:"movements"`
	// AvgDailyConsumption30d is the average units consumed per day over the last 30 days.
	AvgDailyConsumption30d float64 `json:"avgDailyConsumption30d"`
	// EstimatedDaysLeft is nil when there is no consumption to extrapolate from.
	EstimatedDaysLeft *float64 `json:"estimatedDaysLeft"`
}

func (s *itemService) Movements(userID uuid.UUID, id uuid.UUID) (*MovementHistory, error) {
	item, err := s.itemRepo.GetByID(id, userID)
	if err != nil || item == nil {
		return nil, apperr.NotFound("item not found")
	}
	list, err := s.movementRepo.ListByItem(userID, id, 200)
	if err != nil {
		return nil, err
	}
	return buildMovementHistory(list, item.Quantity, time.Now().UTC()), nil
}

func buildMovementHistory(list []models.StockMovement, quantity float64, now time.Time) *MovementHistory {
	since := now.AddDate(0, 0, -30)
	var consumed float64
	for _, m := range list {
		if m.Delta < 0 && m.CreatedAt.After(since) && m.Reason == models.MovementConsume {
			consumed -= m.Delta
		}
	}
	h := &MovementHistory{Movements: list, AvgDailyConsumption30d: consumed / 30}
	if h.AvgDailyConsumption30d > 0 {
		d := quantity / h.AvgDailyConsumption30d
		h.EstimatedDaysLeft = &d
	}
	return h
}

// recordMovement is best-effort: a failed audit write must not fail the stock change.
func (s *itemService) recordMovement(item *models.Item, delta float64, reason, note string) {
	if s.movementRepo == nil {
		return
	}
	m := &models.StockMovement{
		UserID: item.UserID, ItemID: item.ID, Delta: delta,
		QuantityAfter: item.Quantity, Reason: reason, Note: note,
	}
	if err := s.movementRepo.Create(m); err != nil {
		log.Printf("record stock movement failed: %v", err)
	}
}

func (s *itemService) Delete(userID uuid.UUID, id uuid.UUID) error {
	return s.itemRepo.Delete(id, userID)
}
