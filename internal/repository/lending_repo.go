package repository

import (
	"time"

	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LendingRepository interface {
	Create(record *models.LendingRecord) error
	GetByID(id uuid.UUID, userID uuid.UUID) (*models.LendingRecord, error)
	GetActiveLoanByItemID(itemID uuid.UUID) (*models.LendingRecord, error)
	Update(record *models.LendingRecord) error
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
