package repository

import (
	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MovementRepository interface {
	Create(m *models.StockMovement) error
	// ListByItem returns the newest movements first.
	ListByItem(userID, itemID uuid.UUID, limit int) ([]models.StockMovement, error)
}

type movementRepo struct {
	db *gorm.DB
}

func NewMovementRepository(db *gorm.DB) MovementRepository {
	return &movementRepo{db: db}
}

func (r *movementRepo) Create(m *models.StockMovement) error {
	return r.db.Create(m).Error
}

func (r *movementRepo) ListByItem(userID, itemID uuid.UUID, limit int) ([]models.StockMovement, error) {
	var res []models.StockMovement
	err := r.db.Where("user_id = ? AND item_id = ?", userID, itemID).
		Order("created_at DESC, id").Limit(limit).Find(&res).Error
	if res == nil {
		res = []models.StockMovement{}
	}
	return res, err
}
