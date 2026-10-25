package repository

import (
	"fmt"
	"strings"
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
	// UsersWithTrashBefore lists users that have anything trashed before the cutoff.
	UsersWithTrashBefore(cutoff time.Time) ([]uuid.UUID, error)
	// MediaTrashedBefore returns the media URLs of the user's items trashed before
	// the cutoff and not held back by an open loan (those are not purged).
	MediaTrashedBefore(userID uuid.UUID, cutoff time.Time) ([]string, error)
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

// blockedItems returns the trashed items in scope that still have an open loan.
func (r *trashRepo) blockedItems(tx *gorm.DB, userID uuid.UUID, scope func(*gorm.DB) *gorm.DB) ([]models.Item, error) {
	sub := scope(tx.Unscoped().Model(&models.Item{}).Select("id").Where("user_id = ? AND deleted_at IS NOT NULL", userID))
	var blocked []models.Item
	err := tx.Unscoped().Select("id", "name", "box_id").
		Where("user_id = ? AND id IN (?) AND id IN (?)", userID, sub,
			tx.Model(&models.LendingRecord{}).Select("item_id").Where("user_id = ? AND actual_return_date IS NULL", userID)).
		Find(&blocked).Error
	return blocked, err
}

func blockedError(blocked []models.Item) error {
	names := make([]string, 0, 3)
	for i, it := range blocked {
		if i == 3 {
			break
		}
		names = append(names, it.Name)
	}
	msg := "still on loan, mark as returned before deleting permanently: " + strings.Join(names, ", ")
	if len(blocked) > len(names) {
		msg += fmt.Sprintf(" and %d more", len(blocked)-len(names))
	}
	return apperr.Conflict(msg)
}

// purgeItems permanently deletes the trashed items in scope. Never erase the
// only record of who is holding the user's belongings: items with an open loan
// make it fail naming them, or are left in the trash when skipBlocked is set.
// It returns the items that were left behind.
func (r *trashRepo) purgeItems(tx *gorm.DB, userID uuid.UUID, scope func(*gorm.DB) *gorm.DB, skipBlocked bool) ([]models.Item, error) {
	blocked, err := r.blockedItems(tx, userID, scope)
	if err != nil {
		return nil, err
	}
	if len(blocked) > 0 && !skipBlocked {
		return nil, blockedError(blocked)
	}
	skip := make([]uuid.UUID, 0, len(blocked))
	for _, it := range blocked {
		skip = append(skip, it.ID)
	}
	scoped := func(q *gorm.DB) *gorm.DB {
		q = scope(q)
		if len(skip) > 0 {
			q = q.Where("id NOT IN ?", skip)
		}
		return q
	}
	sub := scoped(tx.Unscoped().Model(&models.Item{}).Select("id").Where("user_id = ? AND deleted_at IS NOT NULL", userID))
	if err := tx.Where("user_id = ? AND item_id IN (?)", userID, sub).Delete(&models.LendingRecord{}).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("user_id = ? AND item_id IN (?)", userID, sub).Delete(&models.StockMovement{}).Error; err != nil {
		return nil, err
	}
	err = scoped(tx.Unscoped().Where("user_id = ? AND deleted_at IS NOT NULL", userID)).Delete(&models.Item{}).Error
	return blocked, err
}

func (r *trashRepo) PurgeItem(id, userID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if _, err := r.trashedItem(tx, id, userID); err != nil {
			return err
		}
		_, err := r.purgeItems(tx, userID, func(q *gorm.DB) *gorm.DB { return q.Where("id = ?", id) }, false)
		return err
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
		if _, err := r.purgeItems(tx, userID, func(q *gorm.DB) *gorm.DB { return q.Where("box_id IN ?", ids) }, false); err != nil {
			return err
		}
		return tx.Unscoped().Where("id IN ? AND user_id = ?", ids, userID).Delete(&models.Box{}).Error
	})
}

func (r *trashRepo) UsersWithTrashBefore(cutoff time.Time) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.Raw(`SELECT user_id FROM items WHERE deleted_at IS NOT NULL AND deleted_at < @c
		UNION SELECT user_id FROM boxes WHERE deleted_at IS NOT NULL AND deleted_at < @c`,
		map[string]any{"c": cutoff}).Scan(&ids).Error
	return ids, err
}

func (r *trashRepo) MediaTrashedBefore(userID uuid.UUID, cutoff time.Time) ([]string, error) {
	var items []models.Item
	err := r.db.Unscoped().Select("id", "photos", "receipt_photos").
		Where(`user_id = ? AND deleted_at IS NOT NULL AND deleted_at < ? AND NOT EXISTS (
			SELECT 1 FROM lending_records l WHERE l.item_id = items.id AND l.actual_return_date IS NULL)`,
			userID, cutoff).Find(&items).Error
	if err != nil {
		return nil, err
	}
	var files []string
	for _, it := range items {
		files = append(files, it.Photos...)
		files = append(files, it.ReceiptPhotos...)
	}
	return files, nil
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
		var itemCount int64
		before(tx.Unscoped().Model(&models.Item{}).Where("user_id = ? AND deleted_at IS NOT NULL", userID)).Count(&itemCount)
		// Items still on loan stay in the trash, and so do the boxes that hold them.
		blocked, err := r.purgeItems(tx, userID, before, true)
		if err != nil {
			return err
		}
		keep, err := r.boxesToKeep(tx, userID, blocked)
		if err != nil {
			return err
		}
		del := before(tx.Unscoped().Where("user_id = ? AND deleted_at IS NOT NULL", userID))
		if len(keep) > 0 {
			del = del.Where("id NOT IN ?", keep)
		}
		// One statement: the parent FK is checked at statement end, so child order does not matter.
		res := del.Delete(&models.Box{})
		if res.Error != nil {
			return res.Error
		}
		removed = itemCount - int64(len(blocked)) + res.RowsAffected
		return nil
	})
	return removed, err
}

// boxesToKeep returns the trashed boxes that hold a blocked item, plus all their
// trashed ancestors, so the parent chain of a kept box is never broken.
func (r *trashRepo) boxesToKeep(tx *gorm.DB, userID uuid.UUID, blocked []models.Item) ([]uuid.UUID, error) {
	if len(blocked) == 0 {
		return nil, nil
	}
	var trashed []models.Box
	if err := tx.Unscoped().Select("id", "parent_id").
		Where("user_id = ? AND deleted_at IS NOT NULL", userID).Find(&trashed).Error; err != nil {
		return nil, err
	}
	parent := make(map[uuid.UUID]*uuid.UUID, len(trashed))
	for _, b := range trashed {
		parent[b.ID] = b.ParentID
	}
	seen := map[uuid.UUID]bool{}
	var keep []uuid.UUID
	for _, it := range blocked {
		id := &it.BoxID
		for id != nil && !seen[*id] {
			p, ok := parent[*id]
			if !ok {
				break
			}
			seen[*id] = true
			keep = append(keep, *id)
			id = p
		}
	}
	return keep, nil
}
