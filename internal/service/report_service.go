package service

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ReportService interface {
	GetInventoryHealth(userID uuid.UUID) (*InventoryHealthResponse, error)
	// ExportInfo returns the Content-Type and filename for the given format.
	ExportInfo(format string) (contentType, filename string)
	// ExportItems streams all items of the user to w in batches.
	ExportItems(userID uuid.UUID, format string, w io.Writer) error
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

// exportBatchSize is the page size used to stream items out, so memory stays
// bounded no matter how many items a user owns.
const exportBatchSize = 500

func (s *reportService) ExportInfo(format string) (string, string) {
	stamp := time.Now().Format("20060102_150405")
	if format == "json" {
		return "application/json", fmt.Sprintf("inventory_report_%s.json", stamp)
	}
	return "text/csv", fmt.Sprintf("inventory_report_%s.csv", stamp)
}

// forEachItemBatch walks every item of the user page by page.
func (s *reportService) forEachItemBatch(userID uuid.UUID, fn func([]models.Item) error) error {
	for page := 1; ; page++ {
		items, _, err := s.itemRepo.List(userID, repository.ItemFilter{Page: page, Limit: exportBatchSize})
		if err != nil {
			return err
		}
		if len(items) > 0 {
			if err := fn(items); err != nil {
				return err
			}
		}
		if len(items) < exportBatchSize {
			return nil
		}
	}
}

// csvSafe neutralizes spreadsheet formula injection for user-controlled text.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func (s *reportService) ExportItems(userID uuid.UUID, format string, w io.Writer) error {
	if format == "json" {
		return s.exportJSON(userID, w)
	}
	return s.exportCSV(userID, w)
}

func (s *reportService) exportJSON(userID uuid.UUID, w io.Writer) error {
	if _, err := io.WriteString(w, "["); err != nil {
		return err
	}
	first := true
	err := s.forEachItemBatch(userID, func(items []models.Item) error {
		for _, item := range items {
			data, err := json.MarshalIndent(item, "  ", "  ")
			if err != nil {
				return err
			}
			sep := ",\n  "
			if first {
				sep, first = "\n  ", false
			}
			if _, err := io.WriteString(w, sep); err != nil {
				return err
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	end := "\n]"
	if first {
		end = "]"
	}
	_, err = io.WriteString(w, end)
	return err
}

func (s *reportService) exportCSV(userID uuid.UUID, w io.Writer) error {
	writer := csv.NewWriter(w)

	headers := []string{
		"ID", "Name", "Category", "Quantity", "Unit", "Min Quantity",
		"Status", "Purchase Price", "Purchase Date", "Barcode", "Serial Number",
		"Expiry Date", "Warranty Expiry Date", "Created At",
	}
	if err := writer.Write(headers); err != nil {
		return err
	}

	err := s.forEachItemBatch(userID, func(items []models.Item) error {
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
				csvSafe(item.Name),
				csvSafe(item.CategoryID),
				fmt.Sprintf("%.2f", item.Quantity),
				csvSafe(item.Unit),
				minQty,
				item.Status,
				price,
				purchaseDate,
				csvSafe(barcode),
				csvSafe(serial),
				expiryDate,
				warrantyDate,
				item.CreatedAt.Format(time.RFC3339),
			}
			if err := writer.Write(row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}
