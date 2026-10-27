package repository

import (
	"time"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	// CountsByBox returns item and sub-box counts for every box of the user in two queries.
	CountsByBox(userID uuid.UUID) (items map[uuid.UUID]int64, subBoxes map[uuid.UUID]int64, err error)
	GetBreadcrumbs(boxID uuid.UUID, userID uuid.UUID) ([]models.BreadcrumbItem, error)
	IsDescendant(boxID, potentialDescendantID, userID uuid.UUID) (bool, error)
	// Move re-parents a box (nil = root). The cycle check and the write run in one
	// transaction serialized per user, so concurrent moves cannot create a loop.
	Move(id, userID uuid.UUID, newParentID *uuid.UUID) error
}

// maxBoxDepth bounds recursive tree queries so a corrupt cycle cannot loop forever.
const maxBoxDepth = 64

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

func (r *boxRepo) CountsByBox(userID uuid.UUID) (map[uuid.UUID]int64, map[uuid.UUID]int64, error) {
	type row struct {
		ID    uuid.UUID
		Count int64
	}
	collect := func(rows []row) map[uuid.UUID]int64 {
		m := make(map[uuid.UUID]int64, len(rows))
		for _, r := range rows {
			m[r.ID] = r.Count
		}
		return m
	}

	var itemRows, boxRows []row
	if err := r.db.Model(&models.Item{}).Select("box_id AS id, COUNT(*) AS count").
		Where("user_id = ?", userID).Group("box_id").Scan(&itemRows).Error; err != nil {
		return nil, nil, err
	}
	if err := r.db.Model(&models.Box{}).Select("parent_id AS id, COUNT(*) AS count").
		Where("user_id = ? AND parent_id IS NOT NULL", userID).Group("parent_id").Scan(&boxRows).Error; err != nil {
		return nil, nil, err
	}
	return collect(itemRows), collect(boxRows), nil
}

func (r *boxRepo) Delete(id uuid.UUID, userID uuid.UUID, cascade bool) error {
	deletedAt := time.Now().UTC()
	// Ownership, the emptiness check and the delete share one transaction, so
	// nothing can be added to the box between the check and the delete.
	return r.db.Transaction(func(tx *gorm.DB) error {
		var box models.Box
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).First(&box).Error; err != nil {
			return apperr.NotFoundOr(err, "box not found")
		}

		var itemCount, subBoxCount int64
		if err := tx.Model(&models.Item{}).Where("box_id = ? AND user_id = ?", id, userID).
			Count(&itemCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Box{}).Where("parent_id = ? AND user_id = ?", id, userID).
			Count(&subBoxCount).Error; err != nil {
			return err
		}
		if !cascade && (itemCount > 0 || subBoxCount > 0) {
			return apperr.Conflict("cannot delete box because it contains items or sub-boxes (use cascade=true)")
		}

		if cascade {
			// Find all descendant box IDs recursively
			var descendantIDs []uuid.UUID
			cteQuery := `
				WITH RECURSIVE descendants AS (
					SELECT id FROM boxes WHERE parent_id = ? AND user_id = ? AND deleted_at IS NULL
					UNION ALL
					SELECT b.id FROM boxes b JOIN descendants d ON b.parent_id = d.id WHERE b.user_id = ? AND b.deleted_at IS NULL
				)
				SELECT id FROM descendants;
			`
			if err := tx.Raw(cteQuery, id, userID, userID).Scan(&descendantIDs).Error; err != nil {
				return err
			}

			allBoxIDs := append(descendantIDs, id)

			// Fully lent items have status 'lent'; a partial loan leaves the item
			// 'stored' and is only visible as an open lending record.
			var lentCount int64
			if err := tx.Model(&models.Item{}).
				Where(`box_id IN ? AND user_id = ? AND (status = 'lent' OR EXISTS (
					SELECT 1 FROM lending_records l
					WHERE l.item_id = items.id AND l.actual_return_date IS NULL))`, allBoxIDs, userID).
				Count(&lentCount).Error; err != nil {
				return err
			}
			if lentCount > 0 {
				return apperr.Conflict("cannot delete: some items inside are currently lent out")
			}
			// Soft delete everything with one timestamp so a restore can bring the subtree back together.
			if err := tx.Model(&models.Item{}).Where("box_id IN ? AND user_id = ?", allBoxIDs, userID).
				Update("deleted_at", deletedAt).Error; err != nil {
				return err
			}
			if len(descendantIDs) > 0 {
				if err := tx.Model(&models.Box{}).Where("id IN ? AND user_id = ?", descendantIDs, userID).
					Update("deleted_at", deletedAt).Error; err != nil {
					return err
				}
			}
		}

		return tx.Model(&models.Box{}).Where("id = ? AND user_id = ?", id, userID).
			Update("deleted_at", deletedAt).Error
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
			WHERE p.depth < 64
		)
		SELECT CAST(id AS TEXT) as id, name FROM box_path ORDER BY depth DESC;
	`
	err := r.db.Raw(cteQuery, boxID, userID).Scan(&breadcrumbs).Error
	return breadcrumbs, err
}

func (r *boxRepo) IsDescendant(boxID, potentialDescendantID, userID uuid.UUID) (bool, error) {
	return isDescendant(r.db, boxID, potentialDescendantID, userID)
}

func isDescendant(db *gorm.DB, boxID, potentialDescendantID, userID uuid.UUID) (bool, error) {
	var count int64
	cteQuery := `
		WITH RECURSIVE descendants AS (
			SELECT id, 1 AS depth FROM boxes WHERE parent_id = ? AND user_id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT b.id, d.depth + 1 FROM boxes b JOIN descendants d ON b.parent_id = d.id
			WHERE b.user_id = ? AND b.deleted_at IS NULL AND d.depth < ?
		)
		SELECT COUNT(*) FROM descendants WHERE id = ?;
	`
	err := db.Raw(cteQuery, boxID, userID, userID, maxBoxDepth, potentialDescendantID).Scan(&count).Error
	return count > 0, err
}

func (r *boxRepo) Move(id, userID uuid.UUID, newParentID *uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// One tree per user: serialize all moves of that tree until commit.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", userID.String()).Error; err != nil {
			return err
		}
		var box models.Box
		if err := tx.Where("id = ? AND user_id = ?", id, userID).First(&box).Error; err != nil {
			return apperr.NotFoundOr(err, "box not found")
		}
		if newParentID != nil {
			if *newParentID == id {
				return apperr.Invalid("cannot move a box inside itself")
			}
			var parent models.Box
			if err := tx.Where("id = ? AND user_id = ?", *newParentID, userID).First(&parent).Error; err != nil {
				return apperr.NotFoundOr(err, "target parent box not found")
			}
			isDesc, err := isDescendant(tx, id, *newParentID, userID)
			if err != nil {
				return err
			}
			if isDesc {
				return apperr.Invalid("cannot move a box inside one of its own sub-boxes (cycle detected)")
			}
		}
		return tx.Model(&models.Box{}).Where("id = ? AND user_id = ?", id, userID).
			Updates(map[string]interface{}{"parent_id": newParentID, "updated_at": time.Now().UTC()}).Error
	})
}
