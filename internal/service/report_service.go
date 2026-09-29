package service

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ReportService interface {
	GetInventoryHealth(userID uuid.UUID) (*InventoryHealthResponse, error)
	ExportItems(userID uuid.UUID, format string) ([]byte, string, string, error)
}

type HealthBreakdown struct {
	Stored       int `json:"stored"`
	InUse        int `json:"inUse"`
	Lent         int `json:"lent"`
	LowStock     int `json:"lowStock"`
	Expired      int `json:"expired"`
	ExpiringSoon int `json:"expiringSoon"`
}

type InventoryHealthResponse struct {
	TotalAssetsCount      int             `json:"totalAssetsCount"`
	TotalEstimatedValue   float64         `json:"totalEstimatedValue"`
	Breakdown             HealthBreakdown `json:"breakdown"`
	StorageLocationsCount int             `json:"storageLocationsCount"`
}

type reportService struct {
	db           *gorm.DB
	itemRepo     repository.ItemRepository
	boxRepo      repository.BoxRepository
	settingsRepo repository.SettingsRepository
}

func NewReportService(
	db *gorm.DB,
	itemRepo repository.ItemRepository,
	boxRepo repository.BoxRepository,
	settingsRepo repository.SettingsRepository,
) ReportService {
	return &reportService{
		db:           db,
		itemRepo:     itemRepo,
		boxRepo:      boxRepo,
		settingsRepo: settingsRepo,
	}
}

func (s *reportService) GetInventoryHealth(userID uuid.UUID) (*InventoryHealthResponse, error) {
	totalAssetsCount, err := s.itemRepo.CountByUserID(userID)
	if err != nil {
		return nil, err
	}

	totalEstimatedValue, err := s.itemRepo.GetTotalEstimatedValue(userID)
	if err != nil {
		return nil, err
	}

	var storedCount, inUseCount, lentCount, lowStockCount, expiredCount, expiringSoonCount int64

	s.db.Model(&models.Item{}).Where("user_id = ? AND status = 'stored'", userID).Count(&storedCount)
	s.db.Model(&models.Item{}).Where("user_id = ? AND status = 'inUse'", userID).Count(&inUseCount)
	s.db.Model(&models.Item{}).Where("user_id = ? AND status = 'lent'", userID).Count(&lentCount)
	s.db.Model(&models.Item{}).Where("user_id = ? AND min_quantity IS NOT NULL AND quantity <= min_quantity", userID).Count(&lowStockCount)

	settings, _ := s.settingsRepo.GetByUserID(userID)
	warningDays := 30
	if settings != nil && settings.ExpiryWarningDays > 0 {
		warningDays = settings.ExpiryWarningDays
	}

	today := time.Now().UTC().Format("2006-01-02")
	soonDate := time.Now().UTC().AddDate(0, 0, warningDays).Format("2006-01-02")

	s.db.Model(&models.Item{}).
		Where("user_id = ? AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date < ?", userID, today).
		Count(&expiredCount)

	s.db.Model(&models.Item{}).
		Where("user_id = ? AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date >= ? AND expiry_date <= ?", userID, today, soonDate).
		Count(&expiringSoonCount)

	var storageLocationsCount int64
	s.db.Model(&models.Box{}).Where("user_id = ?", userID).Count(&storageLocationsCount)

	return &InventoryHealthResponse{
		TotalAssetsCount:    int(totalAssetsCount),
		TotalEstimatedValue: totalEstimatedValue,
		Breakdown: HealthBreakdown{
			Stored:       int(storedCount),
			InUse:        int(inUseCount),
			Lent:         int(lentCount),
			LowStock:     int(lowStockCount),
			Expired:      int(expiredCount),
			ExpiringSoon: int(expiringSoonCount),
		},
		StorageLocationsCount: int(storageLocationsCount),
	}, nil
}

func (s *reportService) ExportItems(userID uuid.UUID, format string) ([]byte, string, string, error) {
	items, _, err := s.itemRepo.List(userID, repository.ItemFilter{Limit: 10000})
	if err != nil {
		return nil, "", "", err
	}

	if format == "json" {
		data, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return nil, "", "", err
		}
		filename := fmt.Sprintf("inventory_report_%s.json", time.Now().Format("20060102_150405"))
		return data, "application/json", filename, nil
	}

	// Default to CSV
	buf := new(bytes.Buffer)
	writer := csv.NewWriter(buf)

	// CSV Header
	headers := []string{
		"ID", "Name", "Category", "Quantity", "Unit", "Min Quantity",
		"Status", "Purchase Price", "Purchase Date", "Barcode", "Serial Number",
		"Expiry Date", "Warranty Expiry Date", "Created At",
	}
	if err := writer.Write(headers); err != nil {
		return nil, "", "", err
	}

	for _, item := range items {
		minQty := ""
		if item.MinQuantity != nil {
			minQty = fmt.Sprintf("%.2f", *item.MinQuantity)
		}
		price := ""
		if item.PurchasePrice != nil {
			price = fmt.Sprintf("%.2f", *item.PurchasePrice)
		}
		purchaseDate := ""
		if item.PurchaseDate != nil {
			purchaseDate = *item.PurchaseDate
		}
		barcode := ""
		if item.Barcode != nil {
			barcode = *item.Barcode
		}
		serial := ""
		if item.SerialNumber != nil {
			serial = *item.SerialNumber
		}
		expiryDate := ""
		if item.ExpiryDate != nil {
			expiryDate = *item.ExpiryDate
		}
		warrantyDate := ""
		if item.WarrantyExpiryDate != nil {
			warrantyDate = *item.WarrantyExpiryDate
		}

		row := []string{
			item.ID.String(),
			item.Name,
			item.CategoryID,
			fmt.Sprintf("%.2f", item.Quantity),
			item.Unit,
			minQty,
			item.Status,
			price,
			purchaseDate,
			barcode,
			serial,
			expiryDate,
			warrantyDate,
			item.CreatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(row); err != nil {
			return nil, "", "", err
		}
	}

	writer.Flush()
	filename := fmt.Sprintf("inventory_report_%s.csv", time.Now().Format("20060102_150405"))
	return buf.Bytes(), "text/csv", filename, nil
}
