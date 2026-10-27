package repository

import (
	"fmt"
	"strings"
	"time"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LowStockCond is the SQL form of Item.ComputeIsLowStock. Units out on a partial
// loan still belong to the user, so they count toward the stock level.
const LowStockCond = `min_quantity IS NOT NULL AND quantity + COALESCE((SELECT SUM(l.quantity) FROM lending_records l
	WHERE l.item_id = items.id AND l.is_partial AND l.actual_return_date IS NULL), 0) <= min_quantity`

// LowStockAlertCond is LowStockCond limited to items that are still around:
// disposed, lost and broken items never raise low-stock alerts. The dashboard count,
// the alert list and the isLowStock list filter all use it so their numbers agree.
const LowStockAlertCond = `status NOT IN ('disposed', 'lost', 'broken') AND ` + LowStockCond

type ItemFilter struct {
	Q                 string
	BoxID             *uuid.UUID
	Category          string
	Status            string
	IsLowStock        *bool
	IsExpired         *bool
	IsExpiring        *bool
	ExpiryWarningDays int
	// Today is the user's local calendar date (YYYY-MM-DD); empty means UTC today.
	Today string
	Page  int
	Limit int
}

// QuantityChange is either an absolute Set or a relative Delta (exactly one).
type QuantityChange struct {
	Set    *float64
	Delta  *float64
	Reason string
	Note   string
	// IdempotencyKey, when set, makes a repeated request a no-op that returns the item as is.
	IdempotencyKey string
}

type ItemRepository interface {
	Create(item *models.Item) error
	GetByID(id uuid.UUID, userID uuid.UUID) (*models.Item, error)
	GetByCode(code string, userID *uuid.UUID) (*models.Item, error)
	List(userID uuid.UUID, filter ItemFilter) ([]models.Item, int64, error)
	Update(item *models.Item) error
	Delete(id uuid.UUID, userID uuid.UUID) error
	// ChangeQuantity locks the row, applies the change and writes the stock movement in one
	// transaction. It returns the updated item and the delta actually applied.
	ChangeQuantity(id uuid.UUID, userID uuid.UUID, c QuantityChange) (*models.Item, float64, error)
	Move(id uuid.UUID, userID uuid.UUID, targetBoxID uuid.UUID) error
	CountByUserID(userID uuid.UUID) (int64, error)
	GetTotalEstimatedValue(userID uuid.UUID) (float64, error)
}

type itemRepo struct {
	db *gorm.DB
}

func NewItemRepository(db *gorm.DB) ItemRepository {
	return &itemRepo{db: db}
}

func (r *itemRepo) Create(item *models.Item) error {
	return r.db.Create(item).Error
}

func (r *itemRepo) GetByID(id uuid.UUID, userID uuid.UUID) (*models.Item, error) {
	var item models.Item
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&item).Error
	if err != nil {
		return nil, err
	}
	r.fillLent([]*models.Item{&item})
	return &item, nil
}

// fillLent sets LentQuantity (open partial loans) and re-derives IsLowStock.
func (r *itemRepo) fillLent(items []*models.Item) { FillLent(r.db, items) }

// FillLent is fillLent for callers that load items outside the repository.
func FillLent(db *gorm.DB, items []*models.Item) {
	if len(items) == 0 {
		return
	}
	ids := make([]uuid.UUID, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	var rows []struct {
		ItemID uuid.UUID
		Total  float64
	}
	if err := db.Model(&models.LendingRecord{}).
		Select("item_id, SUM(quantity) AS total").
		Where("item_id IN ? AND is_partial AND actual_return_date IS NULL", ids).
		Group("item_id").Scan(&rows).Error; err != nil {
		return
	}
	lent := make(map[uuid.UUID]float64, len(rows))
	for _, row := range rows {
		lent[row.ItemID] = row.Total
	}
	for _, it := range items {
		it.LentQuantity = lent[it.ID]
		it.IsLowStock = it.ComputeIsLowStock()
	}
}

func (r *itemRepo) GetByCode(code string, userID *uuid.UUID) (*models.Item, error) {
	var item models.Item
	// Every lookup starts from a fresh query: a chained *gorm.DB would carry the id
	// condition of the UUID attempt into the barcode attempt.
	scoped := func() *gorm.DB {
		q := r.db.Session(&gorm.Session{})
		if userID != nil {
			q = q.Where("user_id = ?", *userID)
		}
		return q
	}

	// Try UUID parse first
	if parsedUUID, err := uuid.Parse(code); err == nil {
		var uItem models.Item
		if err := scoped().Where("id = ?", parsedUUID).First(&uItem).Error; err == nil {
			r.fillLent([]*models.Item{&uItem})
			return &uItem, nil
		}
	}

	err := scoped().Where("barcode = ? OR serial_number = ?", code, code).First(&item).Error
	if err != nil {
		return nil, err
	}
	r.fillLent([]*models.Item{&item})
	return &item, nil
}

func (r *itemRepo) List(userID uuid.UUID, filter ItemFilter) ([]models.Item, int64, error) {
	var items []models.Item
	var total int64

	query := r.db.Model(&models.Item{}).Where("user_id = ?", userID)

	if filter.Q != "" {
		searchTerm := "%" + escapeLike(filter.Q) + "%"
		// Names and descriptions ignore case and Vietnamese diacritics ("ban" finds "bàn").
		query = query.Where("(f_unaccent_vi(name) LIKE f_unaccent_vi(?) OR f_unaccent_vi(description) LIKE f_unaccent_vi(?) OR COALESCE(barcode, '') ILIKE ? OR COALESCE(serial_number, '') ILIKE ?)",
			searchTerm, searchTerm, searchTerm, searchTerm)
	}

	if filter.BoxID != nil {
		query = query.Where("box_id = ?", *filter.BoxID)
	}

	if filter.Category != "" {
		query = query.Where("LOWER(category_id) = LOWER(?)", filter.Category)
	}

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	if filter.IsLowStock != nil && *filter.IsLowStock {
		query = query.Where(LowStockAlertCond)
	}

	nowStr := filter.Today
	if nowStr == "" {
		nowStr = time.Now().UTC().Format("2006-01-02")
	}
	if filter.IsExpired != nil && *filter.IsExpired {
		query = query.Where("expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date < ?", nowStr)
	}

	if filter.IsExpiring != nil && *filter.IsExpiring {
		days := filter.ExpiryWarningDays
		if days <= 0 {
			days = 30
		}
		base, _ := time.Parse("2006-01-02", nowStr)
		futureDate := base.AddDate(0, 0, days).Format("2006-01-02")
		query = query.Where("expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date >= ? AND expiry_date <= ?", nowStr, futureDate)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	limit := filter.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	err := query.Order("created_at DESC, id").Limit(limit).Offset(offset).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}

	ptrs := make([]*models.Item, len(items))
	for i := range items {
		ptrs[i] = &items[i]
	}
	r.fillLent(ptrs)

	// Fetch box names for items
	if len(items) > 0 {
		var boxIDs []uuid.UUID
		for _, it := range items {
			boxIDs = append(boxIDs, it.BoxID)
		}
		var boxes []models.Box
		r.db.Where("id IN ?", boxIDs).Select("id, name").Find(&boxes)
		boxMap := make(map[uuid.UUID]string)
		for _, b := range boxes {
			boxMap[b.ID] = b.Name
		}
		for i := range items {
			items[i].BoxName = boxMap[items[i].BoxID]
		}
	}

	return items, total, nil
}

// Update writes the item back only if nobody changed it since it was loaded
// (compared by updated_at); otherwise the caller gets a Conflict instead of
// silently overwriting the other change.
func (r *itemRepo) Update(item *models.Item) error {
	loadedAt := item.UpdatedAt
	item.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	res := r.db.Model(&models.Item{}).
		Where("id = ? AND user_id = ? AND updated_at = ?", item.ID, item.UserID, loadedAt).
		Select("*").Omit("id", "user_id", "created_at").
		Updates(item)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		item.UpdatedAt = loadedAt
		return apperr.Conflict("item was modified by another request, please reload and try again")
	}
	return nil
}

func (r *itemRepo) Delete(id uuid.UUID, userID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var item models.Item
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).
			First(&item).Error; err != nil {
			return apperr.NotFoundOr(err, "item not found")
		}
		if item.Status == "lent" {
			return apperr.Conflict("item is currently lent out; return it before deleting")
		}
		var openLoans int64
		if err := tx.Model(&models.LendingRecord{}).
			Where("item_id = ? AND user_id = ? AND actual_return_date IS NULL", id, userID).Count(&openLoans).Error; err != nil {
			return err
		}
		if openLoans > 0 {
			return apperr.Conflict("item has open loans; return them before deleting")
		}
		// Soft delete: history (loans, movements) stays so the item can be restored.
		return tx.Model(&models.Item{}).Where("id = ? AND user_id = ?", id, userID).
			Update("deleted_at", time.Now().UTC()).Error
	})
}

func (r *itemRepo) ChangeQuantity(id uuid.UUID, userID uuid.UUID, c QuantityChange) (*models.Item, float64, error) {
	var item models.Item
	var applied float64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
			return apperr.NotFoundOr(err, "item not found")
		}
		if c.IdempotencyKey != "" {
			var seen int64
			if err := tx.Model(&models.StockMovement{}).
				Where("user_id = ? AND idempotency_key = ?", userID, c.IdempotencyKey).
				Count(&seen).Error; err != nil {
				return err
			}
			if seen > 0 {
				return nil
			}
		}
		next := item.Quantity
		switch {
		case c.Delta != nil:
			next += *c.Delta
		case c.Set != nil:
			next = *c.Set
		}
		if next < 0 {
			return apperr.Invalid("quantity cannot be negative")
		}
		applied = next - item.Quantity
		if applied == 0 {
			return nil
		}
		item.Quantity = next
		if err := tx.Model(&models.Item{}).Where("id = ? AND user_id = ?", id, userID).
			Updates(map[string]interface{}{"quantity": next, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return tx.Create(&models.StockMovement{UserID: userID, ItemID: id, Delta: applied,
			QuantityAfter: next, Reason: c.Reason, Note: c.Note, IdempotencyKey: c.IdempotencyKey}).Error
	})
	if err != nil {
		return nil, 0, err
	}
	r.fillLent([]*models.Item{&item})
	return &item, applied, nil
}

func (r *itemRepo) Move(id uuid.UUID, userID uuid.UUID, targetBoxID uuid.UUID) error {
	res := r.db.Model(&models.Item{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{
			"box_id":     targetBoxID,
			"updated_at": time.Now().UTC(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperr.NotFound("item not found")
	}
	return nil
}

func (r *itemRepo) CountByUserID(userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&models.Item{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

func (r *itemRepo) GetTotalEstimatedValue(userID uuid.UUID) (float64, error) {
	var total *float64
	row := r.db.Model(&models.Item{}).
		Where("user_id = ? AND purchase_price IS NOT NULL AND status NOT IN ('disposed', 'lost')", userID).
		Select("COALESCE(SUM(purchase_price * quantity), 0)").
		Row()
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("scan error: %w", err)
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

// escapeLike escapes LIKE wildcards so user input is matched literally.
func escapeLike(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
}
