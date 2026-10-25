package repository

import (
	"errors"
	"time"

	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrItemNotFound      = errors.New("item not found")
	ErrItemAlreadyLent   = errors.New("item is already currently lent out")
	ErrItemNotLendable   = errors.New("cannot lend an item with status broken or disposed")
	ErrLoanNotFound      = errors.New("lending record not found")
	ErrLoanAlreadyClosed = errors.New("item has already been returned")
)

type LendingRepository interface {
	Create(record *models.LendingRecord) error
	GetByID(id uuid.UUID, userID uuid.UUID) (*models.LendingRecord, error)
	GetActiveLoanByItemID(itemID uuid.UUID) (*models.LendingRecord, error)
	Update(record *models.LendingRecord) error
	// LendItem atomically checks the item (row-locked), creates the record and marks the item lent.
	LendItem(userID uuid.UUID, record *models.LendingRecord) (*models.Item, error)
	// ReturnLoan atomically closes the record (row-locked) and sets the item back to stored.
	ReturnLoan(userID uuid.UUID, recordID uuid.UUID) error
	ListActive(userID uuid.UUID) ([]models.LendingRecord, error)
	ListOverdue(userID uuid.UUID) ([]models.LendingRecord, error)
	ListHistoryByItemID(userID uuid.UUID, itemID uuid.UUID) ([]models.LendingRecord, error)
	CountActive(userID uuid.UUID) (int64, error)
	CountOverdue(userID uuid.UUID) (int64, error)
}

type lendingRepo struct {
	db *gorm.DB
}

func NewLendingRepository(db *gorm.DB) LendingRepository {
	return &lendingRepo{db: db}
}

func (r *lendingRepo) Create(record *models.LendingRecord) error {
	return r.db.Create(record).Error
}

func (r *lendingRepo) GetByID(id uuid.UUID, userID uuid.UUID) (*models.LendingRecord, error) {
	var record models.LendingRecord
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *lendingRepo) GetActiveLoanByItemID(itemID uuid.UUID) (*models.LendingRecord, error) {
	var record models.LendingRecord
	err := r.db.Where("item_id = ? AND actual_return_date IS NULL", itemID).Order("lent_date DESC").First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *lendingRepo) Update(record *models.LendingRecord) error {
	return r.db.Save(record).Error
}

func (r *lendingRepo) LendItem(userID uuid.UUID, record *models.LendingRecord) (*models.Item, error) {
	var item models.Item
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", record.ItemID, userID).
			First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrItemNotFound
			}
			return err
		}

		switch item.Status {
		case "lent":
			return ErrItemAlreadyLent
		case "broken", "disposed":
			return ErrItemNotLendable
		}

		record.PreviousStatus = item.Status
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Item{}).
			Where("id = ? AND user_id = ?", item.ID, userID).
			Update("status", "lent").Error; err != nil {
			return err
		}
		item.Status = "lent"
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *lendingRepo) ReturnLoan(userID uuid.UUID, recordID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var record models.LendingRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", recordID, userID).
			First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLoanNotFound
			}
			return err
		}
		if record.ActualReturnDate != nil {
			return ErrLoanAlreadyClosed
		}

		now := time.Now().UTC()
		if err := tx.Model(&models.LendingRecord{}).
			Where("id = ? AND user_id = ?", record.ID, userID).
			Update("actual_return_date", now).Error; err != nil {
			return err
		}
		restored := record.PreviousStatus
		if restored == "" || restored == "lent" {
			restored = "stored"
		}
		return tx.Model(&models.Item{}).
			Where("id = ? AND user_id = ?", record.ItemID, userID).
			Update("status", restored).Error
	})
}

func (r *lendingRepo) ListActive(userID uuid.UUID) ([]models.LendingRecord, error) {
	var records []models.LendingRecord
	err := r.db.Where("user_id = ? AND actual_return_date IS NULL", userID).
		Order("lent_date DESC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	r.populateItemInfo(records)
	return records, nil
}

func (r *lendingRepo) ListOverdue(userID uuid.UUID) ([]models.LendingRecord, error) {
	var records []models.LendingRecord
	now := time.Now().UTC()
	err := r.db.Where("user_id = ? AND actual_return_date IS NULL AND expected_return_date IS NOT NULL AND expected_return_date < ?", userID, now).
		Order("expected_return_date ASC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	r.populateItemInfo(records)
	return records, nil
}

func (r *lendingRepo) ListHistoryByItemID(userID uuid.UUID, itemID uuid.UUID) ([]models.LendingRecord, error) {
	var records []models.LendingRecord
	err := r.db.Where("user_id = ? AND item_id = ?", userID, itemID).
		Order("lent_date DESC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	r.populateItemInfo(records)
	return records, nil
}

func (r *lendingRepo) CountActive(userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&models.LendingRecord{}).
		Where("user_id = ? AND actual_return_date IS NULL", userID).
		Count(&count).Error
	return count, err
}

func (r *lendingRepo) CountOverdue(userID uuid.UUID) (int64, error) {
	var count int64
	now := time.Now().UTC()
	err := r.db.Model(&models.LendingRecord{}).
		Where("user_id = ? AND actual_return_date IS NULL AND expected_return_date IS NOT NULL AND expected_return_date < ?", userID, now).
		Count(&count).Error
	return count, err
}

func (r *lendingRepo) populateItemInfo(records []models.LendingRecord) {
	if len(records) == 0 {
		return
	}
	var itemIDs []uuid.UUID
	for _, rec := range records {
		itemIDs = append(itemIDs, rec.ItemID)
	}
	var items []models.Item
	r.db.Where("id IN ?", itemIDs).Select("id, name, barcode").Find(&items)
	itemMap := make(map[uuid.UUID]models.Item)
	for _, it := range items {
		itemMap[it.ID] = it
	}
	for i := range records {
		if it, exists := itemMap[records[i].ItemID]; exists {
			records[i].ItemName = it.Name
			if it.Barcode != nil {
				records[i].ItemCode = *it.Barcode
			}
		}
	}
}
