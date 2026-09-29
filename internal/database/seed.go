package database

import (
	"log"

	"inventory_backend/internal/models"

	"gorm.io/gorm"
)

var defaultCategories = []struct {
	Name     string
	IconName string
}{
	{Name: "Electronics", IconName: "devices"},
	{Name: "Clothing", IconName: "checkroom"},
	{Name: "Tools & Hardware", IconName: "build"},
	{Name: "Kitchen & Dining", IconName: "restaurant"},
	{Name: "Medicine & Health", IconName: "medical_services"},
	{Name: "Documents & Books", IconName: "menu_book"},
	{Name: "Gaming & Media", IconName: "sports_esports"},
	{Name: "Office & Stationery", IconName: "edit_note"},
	{Name: "Personal & Misc", IconName: "category"},
}

func SeedDefaultCategories(db *gorm.DB) error {
	for _, cat := range defaultCategories {
		var count int64
		if err := db.Model(&models.Category{}).Where("name = ? AND is_default = true", cat.Name).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			newCat := models.Category{
				Name:      cat.Name,
				IconName:  cat.IconName,
				IsDefault: true,
				UserID:    nil,
			}
			if err := db.Create(&newCat).Error; err != nil {
				log.Printf("Warning: failed to seed category %s: %v\n", cat.Name, err)
			}
		}
	}
	return nil
}
