package repository

import (
	"time"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TrashRepository manages soft-deleted items and boxes. Only the "roots" of a
// deletion are listed: items and boxes removed together with their parent box
// share its deleted_at and come back or disappear with it.
type TrashRepository interface {
	List(userID uuid.UUID) ([]models.Item, []models.Box, error)
	RestoreItem(id, userID uuid.UUID) error
	RestoreBox(id, userID uuid.UUID) error
	PurgeItem(id, userID uuid.UUID) error
	PurgeBox(id, userID uuid.UUID) error
	// Empty permanently removes everything trashed before the cutoff (all when cutoff is zero).
	Empty(userID uuid.UUID, cutoff time.Time) (int64, error)
}

type trashRepo struct {
	db *gorm.DB
}

func NewTrashRepository(db *gorm.DB) TrashRepository {
	return &trashRepo{db: db}
}

func (r *trashRepo) List(userID uuid.UUID) ([]models.Item, []models.Box, error) {
	items := []models.Item{}
	boxes := []models.Box{}
	if err := r.db.Unscoped().
		Where(`user_id = ? AND deleted_at IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM boxes b WHERE b.id = items.box_id AND b.deleted_at = items.deleted_at)`, userID).
		Order("deleted_at DESC").Find(&items).Error; err != nil {
		return nil, nil, err
	}
	if err := r.db.Unscoped().
		Where(`user_id = ? AND deleted_at IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM boxes p WHERE p.id = boxes.parent_id AND p.deleted_at = boxes.deleted_at)`, userID).
		Order("deleted_at DESC").Find(&boxes).Error; err != nil {
		return nil, nil, err
	}
	return items, boxes, nil
}

func (r *trashRepo) trashedItem(tx *gorm.DB, id, userID uuid.UUID) (*models.Item, error) {
	var item models.Item
	err := tx.Unscoped().Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", id, userID).First(&item).Error
	if err != nil {
		return nil, apperr.NotFoundOr(err, "item not found in trash")
	}
	return &item, nil
}

func (r *trashRepo) trashedBox(tx *gorm.DB, id, userID uuid.UUID) (*models.Box, error) {
	var box models.Box
	err := tx.Unscoped().Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", id, userID).First(&box).Error
	if err != nil {
		return nil, apperr.NotFoundOr(err, "box not found in trash")
	}
	return &box, nil
}

// subtree returns the ids of boxes trashed together with root (same deleted_at), root included.
func (r *trashRepo) subtree(tx *gorm.DB, root *models.Box, userID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := tx.Raw(`
		WITH RECURSIVE sub AS (
			SELECT id FROM boxes WHERE id = ? AND user_id = ?
			UNION ALL
			SELECT b.id FROM boxes b JOIN sub s ON b.parent_id = s.id
			WHERE b.user_id = ? AND b.deleted_at = ?
		)
		SELECT id FROM sub`, root.ID, userID, userID, root.DeletedAt.Time).Scan(&ids).Error
	return ids, err
}

func (r *trashRepo) RestoreItem(id, userID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		item, err := r.trashedItem(tx, id, userID)
		if err != nil {
			return err
		}
		var live int64
		if err := tx.Model(&models.Box{}).Where("id = ? AND user_id = ?", item.BoxID, userID).Count(&live).Error; err != nil {
			return err
		}
		if live == 0 {
			return apperr.Conflict("the box of this item is in the trash or gone; restore the box first")
		}
		return tx.Unscoped().Model(&models.Item{}).Where("id = ? AND user_id = ?", id, userID).
			Updates(map[string]interface{}{"deleted_at": nil, "updated_at": time.Now().UTC()}).Error
	})
}

func (r *trashRepo) RestoreBox(id, userID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		box, err := r.trashedBox(tx, id, userID)
		if err != nil {
			return err
		}
		ids, err := r.subtree(tx, box, userID)
		if err != nil {
			return err
		}
		ts := box.DeletedAt.Time
		now := time.Now().UTC()
		// A trashed parent cannot hold the box: bring it back at the root instead.
		if box.ParentID != nil {
			var live int64
			if err := tx.Model(&models.Box{}).Where("id = ? AND user_id = ?", *box.ParentID, userID).Count(&live).Error; err != nil {
				return err
			}
			if live == 0 {
				if err := tx.Unscoped().Model(&models.Box{}).Where("id = ?", id).Update("parent_id", nil).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Unscoped().Model(&models.Box{}).Where("id IN ? AND user_id = ?", ids, userID).
			Updates(map[string]interface{}{"deleted_at": nil, "updated_at": now}).Error; err != nil {
			return apperr.ConflictOnUnique(err, "a live box already uses the same label")
		}
		return tx.Unscoped().Model(&models.Item{}).
			Where("box_id IN ? AND user_id = ? AND deleted_at = ?", ids, userID, ts).
			Updates(map[string]interface{}{"deleted_at": nil, "updated_at": now}).Error
	})
}

func (r *trashRepo) purgeItems(tx *gorm.DB, userID uuid.UUID, scope func(*gorm.DB) *gorm.DB) error {
	sub := scope(tx.Unscoped().Model(&models.Item{}).Select("id").Where("user_id = ? AND deleted_at IS NOT NULL", userID))
	if err := tx.Where("user_id = ? AND item_id IN (?)", userID, sub).Delete(&models.LendingRecord{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ? AND item_id IN (?)", userID, sub).Delete(&models.StockMovement{}).Error; err != nil {
		return err
	}
	return scope(tx.Unscoped().Where("user_id = ? AND deleted_at IS NOT NULL", userID)).Delete(&models.Item{}).Error
}

func (r *trashRepo) PurgeItem(id, userID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if _, err := r.trashedItem(tx, id, userID); err != nil {
			return err
		}
		return r.purgeItems(tx, userID, func(q *gorm.DB) *gorm.DB { return q.Where("id = ?", id) })
	})
}

func (r *trashRepo) PurgeBox(id, userID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		box, err := r.trashedBox(tx, id, userID)
		if err != nil {
			return err
		}
		ids, err := r.subtree(tx, box, userID)
		if err != nil {
			return err
		}
		if err := r.purgeItems(tx, userID, func(q *gorm.DB) *gorm.DB { return q.Where("box_id IN ?", ids) }); err != nil {
			return err
		}
		return tx.Unscoped().Where("id IN ? AND user_id = ?", ids, userID).Delete(&models.Box{}).Error
	})
}

func (r *trashRepo) Empty(userID uuid.UUID, cutoff time.Time) (int64, error) {
	var removed int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		before := func(q *gorm.DB) *gorm.DB {
			if cutoff.IsZero() {
				return q
			}
			return q.Where("deleted_at < ?", cutoff)
		}
		var itemCount, boxCount int64
		before(tx.Unscoped().Model(&models.Item{}).Where("user_id = ? AND deleted_at IS NOT NULL", userID)).Count(&itemCount)
		before(tx.Unscoped().Model(&models.Box{}).Where("user_id = ? AND deleted_at IS NOT NULL", userID)).Count(&boxCount)
		if err := r.purgeItems(tx, userID, before); err != nil {
			return err
		}
		// One statement: the parent FK is checked at statement end, so child order does not matter.
		if err := before(tx.Unscoped().Where("user_id = ? AND deleted_at IS NOT NULL", userID)).Delete(&models.Box{}).Error; err != nil {
			return err
		}
		removed = itemCount + boxCount
		return nil
	})
	return removed, err
}
