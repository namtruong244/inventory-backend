package repository

import (
	"inventory_backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository interface {
	Create(user *models.User) error
	GetByEmail(email string) (*models.User, error)
	GetByID(id uuid.UUID) (*models.User, error)
	Update(user *models.User) error
	// BumpTokenVersion revokes every JWT issued to the user so far.
	BumpTokenVersion(id uuid.UUID) error
	// DeleteAccount hard-deletes the user and all their data in one transaction
	// and returns the media URLs that were referenced by their items.
	DeleteAccount(id uuid.UUID) ([]string, error)
}

type userRepo struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepo{db: db}
}

func (r *userRepo) Create(user *models.User) error {
	return r.db.Create(user).Error
}

func (r *userRepo) GetByEmail(email string) (*models.User, error) {
	var user models.User
	err := r.db.Where("LOWER(email) = LOWER(?)", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) GetByID(id uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.db.First(&user, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update writes the profile and credential columns only. token_version is left out on
// purpose: it changes through BumpTokenVersion, and a read-modify-write of the whole row
// here could put back an old value and revive tokens that logout-all just revoked.
func (r *userRepo) Update(user *models.User) error {
	return r.db.Model(user).
		Select("name", "email", "password_hash", "provider", "provider_id", "role", "avatar_url", "updated_at").
		Updates(user).Error
}

func (r *userRepo) BumpTokenVersion(id uuid.UUID) error {
	return r.db.Model(&models.User{}).Where("id = ?", id).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error
}

func (r *userRepo) DeleteAccount(id uuid.UUID) ([]string, error) {
	var files []string
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var items []models.Item
		if err := tx.Unscoped().Select("id", "photos", "receipt_photos").
			Where("user_id = ?", id).Find(&items).Error; err != nil {
			return err
		}
		for _, it := range items {
			files = append(files, it.Photos...)
			files = append(files, it.ReceiptPhotos...)
		}
		// Children first; Unscoped also removes rows sitting in the trash.
		for _, m := range []any{&models.LendingRecord{}, &models.StockMovement{}, &models.Item{}, &models.Box{}} {
			if err := tx.Unscoped().Where("user_id = ?", id).Delete(m).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", id).Delete(&models.Category{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&models.UserSettings{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ?", id).Delete(&models.User{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
