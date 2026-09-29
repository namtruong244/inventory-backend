package service

import (
	"errors"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"

	"github.com/google/uuid"
)

type CategoryService interface {
	List(userID uuid.UUID) ([]CategoryResponse, error)
	Create(userID uuid.UUID, name string) (*CategoryResponse, error)
	Delete(userID uuid.UUID, name string) error
}

type CategoryResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

type categoryService struct {
	categoryRepo repository.CategoryRepository
}

func NewCategoryService(categoryRepo repository.CategoryRepository) CategoryService {
	return &categoryService{categoryRepo: categoryRepo}
}

func (s *categoryService) List(userID uuid.UUID) ([]CategoryResponse, error) {
	categories, err := s.categoryRepo.List(userID)
	if err != nil {
		return nil, err
	}

	res := make([]CategoryResponse, 0, len(categories))
	for _, c := range categories {
		res = append(res, CategoryResponse{
			ID:        c.ID.String(),
			Name:      c.Name,
			IsDefault: c.IsDefault,
		})
	}
	return res, nil
}

func (s *categoryService) Create(userID uuid.UUID, name string) (*CategoryResponse, error) {
	if name == "" {
		return nil, errors.New("category name is required")
	}

	existing, _ := s.categoryRepo.GetByName(userID, name)
	if existing != nil {
		return nil, errors.New("category with this name already exists")
	}

	cat := &models.Category{
		UserID:    &userID,
		Name:      name,
		IconName:  "category",
		IsDefault: false,
	}

	if err := s.categoryRepo.Create(cat); err != nil {
		return nil, err
	}

	return &CategoryResponse{
		ID:        cat.ID.String(),
		Name:      cat.Name,
		IsDefault: cat.IsDefault,
	}, nil
}

func (s *categoryService) Delete(userID uuid.UUID, name string) error {
	return s.categoryRepo.DeleteByName(userID, name)
}
