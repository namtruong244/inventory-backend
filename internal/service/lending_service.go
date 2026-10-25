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

	// Business Rule 3: Lending Integrity — checked and applied atomically in the repository
	item, err := s.lendingRepo.LendItem(userID, record)
	if err != nil {
		return nil, err
	}

	record.ItemName = item.Name
	if item.Barcode != nil {
		record.ItemCode = *item.Barcode
	}

	return record, nil
}

func (s *lendingService) ReturnItem(userID uuid.UUID, recordID uuid.UUID) error {
	return s.lendingRepo.ReturnLoan(userID, recordID)
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
