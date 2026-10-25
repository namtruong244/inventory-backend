package service

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/utils"

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

	expiryDays, tz := 30, "UTC"
	if settings, err := s.settingsRepo.GetByUserID(userID); err == nil && settings != nil {
		if settings.ExpiryWarningDays > 0 {
			expiryDays = settings.ExpiryWarningDays
		}
		tz = settings.Timezone
	}
	today := utils.TodayIn(tz)
	soonDate := utils.DateAfter(tz, expiryDays)

	var counts struct {
		Stored, InUse, Lent, LowStock, Expired, ExpiringSoon int64
	}
	// One pass over the user's items instead of six COUNT queries
	if err := s.db.Model(&models.Item{}).
		Select(`COUNT(*) FILTER (WHERE status = 'stored') AS stored,
			COUNT(*) FILTER (WHERE status = 'inUse') AS in_use,
			COUNT(*) FILTER (WHERE status = 'lent') AS lent,
			COUNT(*) FILTER (WHERE min_quantity IS NOT NULL AND quantity <= min_quantity) AS low_stock,
			COUNT(*) FILTER (WHERE expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date < @today) AS expired,
			COUNT(*) FILTER (WHERE expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date >= @today AND expiry_date <= @soon) AS expiring_soon`,
			sql.Named("today", today), sql.Named("soon", soonDate)).
		Where("user_id = ?", userID).
		Scan(&counts).Error; err != nil {
		return nil, err
	}
	storedCount, inUseCount, lentCount := counts.Stored, counts.InUse, counts.Lent
	lowStockCount, expiredCount, expiringSoonCount := counts.LowStock, counts.Expired, counts.ExpiringSoon

	var storageLocationsCount int64
	if err := s.db.Model(&models.Box{}).Where("user_id = ?", userID).Count(&storageLocationsCount).Error; err != nil {
		return nil, err
	}

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
