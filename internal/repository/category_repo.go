package repository

import (
	"fmt"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CategoryRepository interface {
	List(userID uuid.UUID) ([]models.Category, error)
	Create(cat *models.Category) error
	DeleteByName(userID uuid.UUID, name string) error
	GetByName(userID uuid.UUID, name string) (*models.Category, error)
	// Resolve finds a category visible to the user by name (case-insensitive) or by id.
	Resolve(userID uuid.UUID, value string) (*models.Category, error)
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

func (r *categoryRepo) Resolve(userID uuid.UUID, value string) (*models.Category, error) {
	if id, err := uuid.Parse(value); err == nil {
		var cat models.Category
		if err := r.db.Where("id = ? AND (is_default = true OR user_id = ?)", id, userID).First(&cat).Error; err == nil {
			return &cat, nil
		}
	}
	return r.GetByName(userID, value)
}

func (r *categoryRepo) DeleteByName(userID uuid.UUID, name string) error {
	var cat models.Category
	err := r.db.Where("LOWER(name) = LOWER(?) AND (is_default = true OR user_id = ?)", name, userID).First(&cat).Error
	if err != nil {
		return apperr.NotFoundOr(err, "category not found")
	}

	if cat.IsDefault {
		return apperr.Invalid("cannot delete system default category")
	}

	var used int64
	if err := r.db.Model(&models.Item{}).
		Where("user_id = ? AND LOWER(category_id) = LOWER(?)", userID, cat.Name).
		Count(&used).Error; err != nil {
		return err
	}
	if used > 0 {
		return apperr.Conflict(fmt.Sprintf("category is used by %d item(s); change their category first", used))
	}

	return r.db.Delete(&cat).Error
}
