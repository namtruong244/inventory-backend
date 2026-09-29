package service

import (
	"errors"
	"time"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"

	"github.com/google/uuid"
)

type LendingService interface {
	LendItem(userID uuid.UUID, req LendItemRequest) (*models.LendingRecord, error)
	ReturnItem(userID uuid.UUID, recordID uuid.UUID) error
	GetActiveLoans(userID uuid.UUID) ([]models.LendingRecord, error)
	GetOverdueLoans(userID uuid.UUID) ([]models.LendingRecord, error)
	GetHistoryByItemID(userID uuid.UUID, itemID uuid.UUID) ([]models.LendingRecord, error)
}

type LendItemRequest struct {
	ItemID             uuid.UUID  `json:"itemId" binding:"required"`
	BorrowerName       string     `json:"borrowerName" binding:"required"`
	BorrowerContact    string     `json:"borrowerContact"`
	LentDate           *time.Time `json:"lentDate"`
	ExpectedReturnDate *time.Time `json:"expectedReturnDate"`
	Notes              string     `json:"notes"`
}

type lendingService struct {
	lendingRepo repository.LendingRepository
	itemRepo    repository.ItemRepository
}

func NewLendingService(lendingRepo repository.LendingRepository, itemRepo repository.ItemRepository) LendingService {
	return &lendingService{
		lendingRepo: lendingRepo,
		itemRepo:    itemRepo,
	}
}

func (s *lendingService) LendItem(userID uuid.UUID, req LendItemRequest) (*models.LendingRecord, error) {
	item, err := s.itemRepo.GetByID(req.ItemID, userID)
	if err != nil || item == nil {
		return nil, errors.New("item not found")
	}

	// Business Rule 3: Lending Integrity
	if item.Status == "lent" {
		return nil, errors.New("item is already currently lent out")
	}
	if item.Status == "broken" || item.Status == "disposed" {
		return nil, errors.New("cannot lend an item with status broken or disposed")
	}

	lentDate := time.Now().UTC()
	if req.LentDate != nil {
		lentDate = *req.LentDate
	}

	record := &models.LendingRecord{
		UserID:             userID,
		ItemID:             req.ItemID,
		BorrowerName:       req.BorrowerName,
		BorrowerContact:    req.BorrowerContact,
		LentDate:           lentDate,
		ExpectedReturnDate: req.ExpectedReturnDate,
		Notes:              req.Notes,
	}

	if err := s.lendingRepo.Create(record); err != nil {
		return nil, err
	}

	// Update item status to 'lent'
	item.Status = "lent"
	if err := s.itemRepo.Update(item); err != nil {
		return nil, err
	}

	record.ItemName = item.Name
	if item.Barcode != nil {
		record.ItemCode = *item.Barcode
	}

	return record, nil
}

func (s *lendingService) ReturnItem(userID uuid.UUID, recordID uuid.UUID) error {
	record, err := s.lendingRepo.GetByID(recordID, userID)
	if err != nil || record == nil {
		return errors.New("lending record not found")
	}

	if record.ActualReturnDate != nil {
		return errors.New("item has already been returned")
	}

	now := time.Now().UTC()
	record.ActualReturnDate = &now
	if err := s.lendingRepo.Update(record); err != nil {
		return err
	}

	// Update item status back to 'stored'
	item, err := s.itemRepo.GetByID(record.ItemID, userID)
	if err == nil && item != nil {
		item.Status = "stored"
		_ = s.itemRepo.Update(item)
	}

	return nil
}

func (s *lendingService) GetActiveLoans(userID uuid.UUID) ([]models.LendingRecord, error) {
	return s.lendingRepo.ListActive(userID)
}

func (s *lendingService) GetOverdueLoans(userID uuid.UUID) ([]models.LendingRecord, error) {
	return s.lendingRepo.ListOverdue(userID)
}

func (s *lendingService) GetHistoryByItemID(userID uuid.UUID, itemID uuid.UUID) ([]models.LendingRecord, error) {
	// Verify item belongs to user
	_, err := s.itemRepo.GetByID(itemID, userID)
	if err != nil {
		return nil, errors.New("item not found")
	}
	return s.lendingRepo.ListHistoryByItemID(userID, itemID)
}
