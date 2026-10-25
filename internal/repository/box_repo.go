package repository

import (
	"time"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BoxRepository interface {
	Create(box *models.Box) error
	GetByID(id uuid.UUID, userID uuid.UUID) (*models.Box, error)
	GetByLabel(label string, userID *uuid.UUID) (*models.Box, error)
	ListByParent(userID uuid.UUID, parentID *uuid.UUID) ([]models.Box, error)
	ListAll(userID uuid.UUID) ([]models.Box, error)
	Update(box *models.Box) error
	Delete(id uuid.UUID, userID uuid.UUID, cascade bool) error
	CountItemsInBox(boxID, userID uuid.UUID) (int64, error)
	CountSubBoxesInBox(boxID, userID uuid.UUID) (int64, error)
	GetBreadcrumbs(boxID uuid.UUID, userID uuid.UUID) ([]models.BreadcrumbItem, error)
	IsDescendant(boxID, potentialDescendantID, userID uuid.UUID) (bool, error)
}

type boxRepo struct {
	db *gorm.DB
}

func NewBoxRepository(db *gorm.DB) BoxRepository {
	return &boxRepo{db: db}
}

func (r *boxRepo) Create(box *models.Box) error {
	return r.db.Create(box).Error
}

func (r *boxRepo) GetByID(id uuid.UUID, userID uuid.UUID) (*models.Box, error) {
	var box models.Box
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&box).Error
	if err != nil {
		return nil, err
	}
	return &box, nil
}

func (r *boxRepo) GetByLabel(label string, userID *uuid.UUID) (*models.Box, error) {
	var box models.Box
	query := r.db.Where("label = ?", label)
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	err := query.First(&box).Error
	if err != nil {
		return nil, err
	}
	return &box, nil
}

func (r *boxRepo) ListByParent(userID uuid.UUID, parentID *uuid.UUID) ([]models.Box, error) {
	var boxes []models.Box
	query := r.db.Where("user_id = ?", userID)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	err := query.Order("name ASC").Find(&boxes).Error
	return boxes, err
}

func (r *boxRepo) ListAll(userID uuid.UUID) ([]models.Box, error) {
	var boxes []models.Box
	err := r.db.Where("user_id = ?", userID).Order("name ASC").Find(&boxes).Error
	return boxes, err
}

// Update uses optimistic locking on updated_at, see itemRepo.Update.
func (r *boxRepo) Update(box *models.Box) error {
	loadedAt := box.UpdatedAt
	box.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	res := r.db.Model(&models.Box{}).
		Where("id = ? AND user_id = ? AND updated_at = ?", box.ID, box.UserID, loadedAt).
		Select("*").Omit("id", "user_id", "created_at").
		Updates(box)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		box.UpdatedAt = loadedAt
		return apperr.Conflict("box was modified by another request, please reload and try again")
	}
	return nil
}

func (r *boxRepo) CountItemsInBox(boxID, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&models.Item{}).Where("box_id = ? AND user_id = ?", boxID, userID).Count(&count).Error
	return count, err
}

func (r *boxRepo) CountSubBoxesInBox(boxID, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&models.Box{}).Where("parent_id = ? AND user_id = ?", boxID, userID).Count(&count).Error
	return count, err
}

func (r *boxRepo) Delete(id uuid.UUID, userID uuid.UUID, cascade bool) error {
	// Ownership must be verified first: every later query is scoped to this user too.
	if _, err := r.GetByID(id, userID); err != nil {
		return err
	}

	itemCount, err := r.CountItemsInBox(id, userID)
	if err != nil {
		return err
	}
	subBoxCount, err := r.CountSubBoxesInBox(id, userID)
	if err != nil {
		return err
	}

	if !cascade && (itemCount > 0 || subBoxCount > 0) {
		return apperr.Conflict("cannot delete box because it contains items or sub-boxes (use cascade=true)")
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		if cascade {
			// Find all descendant box IDs recursively
			var descendantIDs []uuid.UUID
			cteQuery := `
				WITH RECURSIVE descendants AS (
					SELECT id FROM boxes WHERE parent_id = ? AND user_id = ?
					UNION ALL
					SELECT b.id FROM boxes b JOIN descendants d ON b.parent_id = d.id WHERE b.user_id = ?
				)
				SELECT id FROM descendants;
			`
			if err := tx.Raw(cteQuery, id, userID, userID).Scan(&descendantIDs).Error; err != nil {
				return err
			}

			allBoxIDs := append(descendantIDs, id)

			var lentCount int64
			if err := tx.Model(&models.Item{}).
				Where("box_id IN ? AND user_id = ? AND status = 'lent'", allBoxIDs, userID).
				Count(&lentCount).Error; err != nil {
				return err
			}
			if lentCount > 0 {
				return apperr.Conflict("cannot delete: some items inside are currently lent out")
			}
			if err := tx.Where("user_id = ? AND item_id IN (?)", userID,
				tx.Model(&models.Item{}).Select("id").Where("box_id IN ? AND user_id = ?", allBoxIDs, userID)).
				Delete(&models.LendingRecord{}).Error; err != nil {
				return err
			}

			// Delete items in these boxes
			if err := tx.Where("box_id IN ? AND user_id = ?", allBoxIDs, userID).Delete(&models.Item{}).Error; err != nil {
				return err
			}

			// Delete descendant boxes
			if len(descendantIDs) > 0 {
				if err := tx.Where("id IN ? AND user_id = ?", descendantIDs, userID).Delete(&models.Box{}).Error; err != nil {
					return err
				}
			}
		}

		// Delete the target box
		return tx.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Box{}).Error
	})
}

func (r *boxRepo) GetBreadcrumbs(boxID uuid.UUID, userID uuid.UUID) ([]models.BreadcrumbItem, error) {
	var breadcrumbs []models.BreadcrumbItem
	cteQuery := `
		WITH RECURSIVE box_path AS (
			SELECT id, name, parent_id, 1 as depth
			FROM boxes
			WHERE id = ? AND user_id = ?
			UNION ALL
			SELECT b.id, b.name, b.parent_id, p.depth + 1
			FROM boxes b
			JOIN box_path p ON b.id = p.parent_id
		)
		SELECT CAST(id AS TEXT) as id, name FROM box_path ORDER BY depth DESC;
	`
	err := r.db.Raw(cteQuery, boxID, userID).Scan(&breadcrumbs).Error
	return breadcrumbs, err
}

func (r *boxRepo) IsDescendant(boxID, potentialDescendantID, userID uuid.UUID) (bool, error) {
	var count int64
	cteQuery := `
		WITH RECURSIVE descendants AS (
			SELECT id FROM boxes WHERE parent_id = ? AND user_id = ?
			UNION ALL
			SELECT b.id FROM boxes b JOIN descendants d ON b.parent_id = d.id WHERE b.user_id = ?
		)
		SELECT COUNT(*) FROM descendants WHERE id = ?;
	`
	err := r.db.Raw(cteQuery, boxID, userID, userID, potentialDescendantID).Scan(&count).Error
	return count > 0, err
}
