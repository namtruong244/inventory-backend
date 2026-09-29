package service

import (
	"time"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AlertService interface {
	GetSummary(userID uuid.UUID) (*AlertSummaryResponse, error)
	GetAll(userID uuid.UUID) (*AlertAllResponse, error)
}

type AlertSummaryResponse struct {
	TotalAlertCount     int `json:"totalAlertCount"`
	ExpiredCount        int `json:"expiredCount"`
	ExpiringSoonCount   int `json:"expiringSoonCount"`
	WarrantyEndingCount int `json:"warrantyEndingCount"`
	OverdueLoansCount   int `json:"overdueLoansCount"`
	LowStockCount       int `json:"lowStockCount"`
}

type AlertAllResponse struct {
	ExpiredItems       []models.Item          `json:"expiredItems"`
	ExpiringSoonItems  []models.Item          `json:"expiringSoonItems"`
	ExpiringWarranties []models.Item          `json:"expiringWarranties"`
	OverdueLoans       []models.LendingRecord `json:"overdueLoans"`
	LowStockItems      []models.Item          `json:"lowStockItems"`
}

type alertService struct {
	db           *gorm.DB
	settingsRepo repository.SettingsRepository
	lendingRepo  repository.LendingRepository
}

func NewAlertService(db *gorm.DB, settingsRepo repository.SettingsRepository, lendingRepo repository.LendingRepository) AlertService {
	return &alertService{
		db:           db,
		settingsRepo: settingsRepo,
		lendingRepo:  lendingRepo,
	}
}

func (s *alertService) GetSummary(userID uuid.UUID) (*AlertSummaryResponse, error) {
	settings, _ := s.settingsRepo.GetByUserID(userID)
	warningDays := 30
	if settings != nil && settings.ExpiryWarningDays > 0 {
		warningDays = settings.ExpiryWarningDays
	}

	today := time.Now().UTC().Format("2006-01-02")
	soonDate := time.Now().UTC().AddDate(0, 0, warningDays).Format("2006-01-02")
	warrantySoonDate := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")

	var expiredCount, expiringSoonCount, warrantyEndingCount, lowStockCount int64

	// Expired items
	s.db.Model(&models.Item{}).
		Where("user_id = ? AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date < ?", userID, today).
		Count(&expiredCount)

	// Expiring soon items
	s.db.Model(&models.Item{}).
		Where("user_id = ? AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date >= ? AND expiry_date <= ?", userID, today, soonDate).
		Count(&expiringSoonCount)

	// Warranty ending soon items
	s.db.Model(&models.Item{}).
		Where("user_id = ? AND warranty_expiry_date IS NOT NULL AND warranty_expiry_date != '' AND warranty_expiry_date >= ? AND warranty_expiry_date <= ?", userID, today, warrantySoonDate).
		Count(&warrantyEndingCount)

	// Low stock items
	s.db.Model(&models.Item{}).
		Where("user_id = ? AND min_quantity IS NOT NULL AND quantity <= min_quantity", userID).
		Count(&lowStockCount)

	// Overdue loans
	overdueLoansCount, _ := s.lendingRepo.CountOverdue(userID)

	totalAlerts := int(expiredCount + expiringSoonCount + warrantyEndingCount + overdueLoansCount + lowStockCount)

	return &AlertSummaryResponse{
		TotalAlertCount:     totalAlerts,
		ExpiredCount:        int(expiredCount),
		ExpiringSoonCount:   int(expiringSoonCount),
		WarrantyEndingCount: int(warrantyEndingCount),
		OverdueLoansCount:   int(overdueLoansCount),
		LowStockCount:       int(lowStockCount),
	}, nil
}

func (s *alertService) GetAll(userID uuid.UUID) (*AlertAllResponse, error) {
	settings, _ := s.settingsRepo.GetByUserID(userID)
	warningDays := 30
	if settings != nil && settings.ExpiryWarningDays > 0 {
		warningDays = settings.ExpiryWarningDays
	}

	today := time.Now().UTC().Format("2006-01-02")
	soonDate := time.Now().UTC().AddDate(0, 0, warningDays).Format("2006-01-02")
	warrantySoonDate := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")

	var expiredItems []models.Item
	s.db.Where("user_id = ? AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date < ?", userID, today).
		Order("expiry_date ASC").Find(&expiredItems)

	var expiringSoonItems []models.Item
	s.db.Where("user_id = ? AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date >= ? AND expiry_date <= ?", userID, today, soonDate).
		Order("expiry_date ASC").Find(&expiringSoonItems)

	var warrantyEndingItems []models.Item
	s.db.Where("user_id = ? AND warranty_expiry_date IS NOT NULL AND warranty_expiry_date != '' AND warranty_expiry_date >= ? AND warranty_expiry_date <= ?", userID, today, warrantySoonDate).
		Order("warranty_expiry_date ASC").Find(&warrantyEndingItems)

	var lowStockItems []models.Item
	s.db.Where("user_id = ? AND min_quantity IS NOT NULL AND quantity <= min_quantity", userID).
		Order("quantity ASC").Find(&lowStockItems)

	overdueLoans, err := s.lendingRepo.ListOverdue(userID)
	if err != nil {
		overdueLoans = []models.LendingRecord{}
	}

	if expiredItems == nil {
		expiredItems = []models.Item{}
	}
	if expiringSoonItems == nil {
		expiringSoonItems = []models.Item{}
	}
	if warrantyEndingItems == nil {
		warrantyEndingItems = []models.Item{}
	}
	if lowStockItems == nil {
		lowStockItems = []models.Item{}
	}

	return &AlertAllResponse{
		ExpiredItems:       expiredItems,
		ExpiringSoonItems:  expiringSoonItems,
		ExpiringWarranties: warrantyEndingItems,
		OverdueLoans:       overdueLoans,
		LowStockItems:      lowStockItems,
	}, nil
}
