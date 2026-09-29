package repository

import (
	"errors"

	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CategoryRepository interface {
	List(userID uuid.UUID) ([]models.Category, error)
	Create(cat *models.Category) error
	DeleteByName(userID uuid.UUID, name string) error
	GetByName(userID uuid.UUID, name string) (*models.Category, error)
}

type categoryRepo struct {
	db *gorm.DB
}

func NewCategoryRepository(db *gorm.DB) CategoryRepository {
	return &categoryRepo{db: db}
}

func (r *categoryRepo) List(userID uuid.UUID) ([]models.Category, error) {
	var categories []models.Category
	err := r.db.Where("is_default = true OR user_id = ?", userID).
		Order("is_default DESC, name ASC").
		Find(&categories).Error
	return categories, err
}

func (r *categoryRepo) Create(cat *models.Category) error {
	return r.db.Create(cat).Error
}

func (r *categoryRepo) GetByName(userID uuid.UUID, name string) (*models.Category, error) {
	var cat models.Category
	err := r.db.Where("LOWER(name) = LOWER(?) AND (is_default = true OR user_id = ?)", name, userID).First(&cat).Error
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

func (r *categoryRepo) DeleteByName(userID uuid.UUID, name string) error {
	var cat models.Category
	err := r.db.Where("LOWER(name) = LOWER(?) AND (is_default = true OR user_id = ?)", name, userID).First(&cat).Error
	if err != nil {
		return err
	}

	if cat.IsDefault {
		return errors.New("cannot delete system default category")
	}

	return r.db.Delete(&cat).Error
}
