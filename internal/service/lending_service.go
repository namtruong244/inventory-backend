package service

import (
	"errors"
	"time"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/utils"

	"github.com/google/uuid"
)

type LendingService interface {
	LendItem(userID uuid.UUID, req LendItemRequest) (*models.LendingRecord, error)
	ReturnItem(userID uuid.UUID, recordID uuid.UUID) error
	GetActiveLoans(userID uuid.UUID) ([]models.LendingRecord, error)
	GetOverdueLoans(userID uuid.UUID) ([]models.LendingRecord, error)
	GetHistoryByItemID(userID uuid.UUID, itemID uuid.UUID) ([]models.LendingRecord, error)
	// GetReturnedLoans lists closed loans of every item, newest return first (paginated).
	GetReturnedLoans(userID uuid.UUID, page, limit int) ([]models.LendingRecord, int64, error)
}

type LendItemRequest struct {
	// ID is optional: offline clients pre-generate it so a replayed lend is idempotent.
	ID                 *uuid.UUID `json:"id"`
	ItemID             uuid.UUID  `json:"itemId" binding:"required"`
	BorrowerName       string     `json:"borrowerName" binding:"required"`
	BorrowerContact    string     `json:"borrowerContact"`
	LentDate           *time.Time `json:"lentDate"`
	ExpectedReturnDate *time.Time `json:"expectedReturnDate"`
	Notes              string     `json:"notes"`
	// Quantity lends only part of the stock (0 < q < item quantity); omit to lend the whole item.
	Quantity *float64 `json:"quantity"`
}

type lendingService struct {
	lendingRepo  repository.LendingRepository
	itemRepo     repository.ItemRepository
	settingsRepo repository.SettingsRepository
}

// todayFor is the user's local calendar date (YYYY-MM-DD) from their timezone setting.
func todayFor(settingsRepo repository.SettingsRepository, userID uuid.UUID) string {
	tz := "UTC"
	if settingsRepo != nil {
		if st, err := settingsRepo.GetByUserID(userID); err == nil && st != nil {
			tz = st.Timezone
		}
	}
	return utils.TodayIn(tz)
}

func NewLendingService(lendingRepo repository.LendingRepository, itemRepo repository.ItemRepository, settingsRepo repository.SettingsRepository) LendingService {
	return &lendingService{
		lendingRepo:  lendingRepo,
		itemRepo:     itemRepo,
		settingsRepo: settingsRepo,
	}
}

func (s *lendingService) LendItem(userID uuid.UUID, req LendItemRequest) (*models.LendingRecord, error) {
	if req.ID != nil {
		if existing, err := s.lendingRepo.GetByID(*req.ID, userID); err == nil && existing != nil {
			return existing, nil
		}
	}

	lentDate := time.Now().UTC()
	if req.LentDate != nil {
		lentDate = *req.LentDate
	}

	// Compare by day: clients send a date (midnight) for "expected return".
	lentDay := time.Date(lentDate.Year(), lentDate.Month(), lentDate.Day(), 0, 0, 0, 0, lentDate.Location())
	if req.ExpectedReturnDate != nil && req.ExpectedReturnDate.Before(lentDay) {
		return nil, apperr.Invalid("expectedReturnDate cannot be before lentDate")
	}

	var quantity float64
	if req.Quantity != nil {
		if *req.Quantity <= 0 {
			return nil, apperr.Invalid("quantity must be greater than 0")
		}
		quantity = *req.Quantity
	}

	record := &models.LendingRecord{
		Quantity:           quantity,
		UserID:             userID,
		ItemID:             req.ItemID,
		BorrowerName:       req.BorrowerName,
		BorrowerContact:    req.BorrowerContact,
		LentDate:           lentDate,
		ExpectedReturnDate: req.ExpectedReturnDate,
		Notes:              req.Notes,
	}

	if req.ID != nil {
		record.ID = *req.ID
	}

	// Business Rule 3: Lending Integrity — checked and applied atomically in the repository
	item, err := s.lendingRepo.LendItem(userID, record)
	if err != nil {
		// A client-chosen id may already belong to another user's record.
		if apperr.IsUnique(err) {
			return nil, apperr.Conflict("a lending record with this id already exists")
		}
		return nil, translateLendingErr(err)
	}

	record.ItemName = item.Name
	if item.Barcode != nil {
		record.ItemCode = *item.Barcode
	}

	return record, nil
}

func (s *lendingService) ReturnItem(userID uuid.UUID, recordID uuid.UUID) error {
	return translateLendingErr(s.lendingRepo.ReturnLoan(userID, recordID))
}

// translateLendingErr converts repository sentinels into typed business errors.
func translateLendingErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrItemNotFound), errors.Is(err, repository.ErrLoanNotFound):
		return apperr.NotFound(err.Error())
	case errors.Is(err, repository.ErrItemAlreadyLent), errors.Is(err, repository.ErrLoanAlreadyClosed):
		return apperr.Conflict(err.Error())
	case errors.Is(err, repository.ErrItemNotLendable), errors.Is(err, repository.ErrQuantityExceeds):
		return apperr.Invalid(err.Error())
	}
	return err
}

func (s *lendingService) GetActiveLoans(userID uuid.UUID) ([]models.LendingRecord, error) {
	return s.lendingRepo.ListActive(userID)
}

func (s *lendingService) GetOverdueLoans(userID uuid.UUID) ([]models.LendingRecord, error) {
	return s.lendingRepo.ListOverdue(userID, todayFor(s.settingsRepo, userID))
}

func (s *lendingService) GetReturnedLoans(userID uuid.UUID, page, limit int) ([]models.LendingRecord, int64, error) {
	return s.lendingRepo.ListReturned(userID, page, limit)
}

func (s *lendingService) GetHistoryByItemID(userID uuid.UUID, itemID uuid.UUID) ([]models.LendingRecord, error) {
	// Verify item belongs to user
	_, err := s.itemRepo.GetByID(itemID, userID)
	if err != nil {
		return nil, apperr.NotFound("item not found")
	}
	return s.lendingRepo.ListHistoryByItemID(userID, itemID)
}
