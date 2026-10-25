package service

import (
	"database/sql"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AlertService interface {
	GetSummary(userID uuid.UUID) (*AlertSummaryResponse, error)
	GetAll(userID uuid.UUID) (*AlertAllResponse, error)
}

type AlertSummaryResponse struct {
	TotalAlerts           int `json:"totalAlerts"`
	OverdueCount          int `json:"overdueCount"`
	ExpiredCount          int `json:"expiredCount"`
	ExpiringSoonCount     int `json:"expiringSoonCount"`
	LowStockCount         int `json:"lowStockCount"`
	ExpiringWarrantyCount int `json:"expiringWarrantyCount"`

	// Backward compatibility aliases
	TotalAlertCount     int `json:"totalAlertCount"`
	OverdueLoansCount   int `json:"overdueLoansCount"`
	WarrantyEndingCount int `json:"warrantyEndingCount"`
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

// Items that are gone (disposed/lost) never raise alerts; broken ones still
// carry a warranty but need neither restocking nor an expiry reminder.
const (
	warrantyLive = "status NOT IN ('disposed', 'lost')"
	expiryLive   = "status NOT IN ('disposed', 'lost', 'broken')"
)

// alertWindow holds the date boundaries (YYYY-MM-DD, in the user's timezone) used by every alert query.
type alertWindow struct {
	today, expirySoon, warrantySoon string
}

func (s *alertService) window(userID uuid.UUID) alertWindow {
	expiryDays, warrantyDays, tz := 30, 30, "UTC"
	if settings, err := s.settingsRepo.GetByUserID(userID); err == nil && settings != nil {
		if settings.ExpiryWarningDays > 0 {
			expiryDays = settings.ExpiryWarningDays
		}
		if settings.WarrantyWarningDays > 0 {
			warrantyDays = settings.WarrantyWarningDays
		}
		tz = settings.Timezone
	}
	return alertWindow{
		today:        utils.TodayIn(tz),
		expirySoon:   utils.DateAfter(tz, expiryDays),
		warrantySoon: utils.DateAfter(tz, warrantyDays),
	}
}

func (s *alertService) GetSummary(userID uuid.UUID) (*AlertSummaryResponse, error) {
	w := s.window(userID)

	var counts struct {
		Expired      int64
		ExpiringSoon int64
		Warranty     int64
		LowStock     int64
	}
	// One pass over the user's items instead of one COUNT query per alert type
	err := s.db.Model(&models.Item{}).
		Select(`COUNT(*) FILTER (WHERE `+expiryLive+` AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date < @today) AS expired,
			COUNT(*) FILTER (WHERE `+expiryLive+` AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date >= @today AND expiry_date <= @soon) AS expiring_soon,
			COUNT(*) FILTER (WHERE `+warrantyLive+` AND warranty_expiry_date IS NOT NULL AND warranty_expiry_date != '' AND warranty_expiry_date >= @today AND warranty_expiry_date <= @warranty) AS warranty,
			COUNT(*) FILTER (WHERE `+expiryLive+` AND `+repository.LowStockCond+`) AS low_stock`,
			sql.Named("today", w.today), sql.Named("soon", w.expirySoon), sql.Named("warranty", w.warrantySoon)).
		Where("user_id = ?", userID).
		Scan(&counts).Error
	if err != nil {
		return nil, err
	}

	overdueLoansCount, err := s.lendingRepo.CountOverdue(userID, w.today)
	if err != nil {
		return nil, err
	}

	totalAlerts := int(counts.Expired + counts.ExpiringSoon + counts.Warranty + overdueLoansCount + counts.LowStock)

	return &AlertSummaryResponse{
		TotalAlerts:           totalAlerts,
		OverdueCount:          int(overdueLoansCount),
		ExpiredCount:          int(counts.Expired),
		ExpiringSoonCount:     int(counts.ExpiringSoon),
		LowStockCount:         int(counts.LowStock),
		ExpiringWarrantyCount: int(counts.Warranty),

		// Backward compatibility
		TotalAlertCount:     totalAlerts,
		OverdueLoansCount:   int(overdueLoansCount),
		WarrantyEndingCount: int(counts.Warranty),
	}, nil
}

func (s *alertService) GetAll(userID uuid.UUID) (*AlertAllResponse, error) {
	w := s.window(userID)

	expiredItems := []models.Item{}
	if err := s.db.Where("user_id = ? AND "+expiryLive+" AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date < ?", userID, w.today).
		Order("expiry_date ASC").Find(&expiredItems).Error; err != nil {
		return nil, err
	}

	expiringSoonItems := []models.Item{}
	if err := s.db.Where("user_id = ? AND "+expiryLive+" AND expiry_date IS NOT NULL AND expiry_date != '' AND expiry_date >= ? AND expiry_date <= ?", userID, w.today, w.expirySoon).
		Order("expiry_date ASC").Find(&expiringSoonItems).Error; err != nil {
		return nil, err
	}

	warrantyEndingItems := []models.Item{}
	if err := s.db.Where("user_id = ? AND "+warrantyLive+" AND warranty_expiry_date IS NOT NULL AND warranty_expiry_date != '' AND warranty_expiry_date >= ? AND warranty_expiry_date <= ?", userID, w.today, w.warrantySoon).
		Order("warranty_expiry_date ASC").Find(&warrantyEndingItems).Error; err != nil {
		return nil, err
	}

	lowStockItems := []models.Item{}
	if err := s.db.Where("user_id = ? AND "+expiryLive+" AND "+repository.LowStockCond, userID).
		Order("quantity ASC").Find(&lowStockItems).Error; err != nil {
		return nil, err
	}
	ptrs := make([]*models.Item, len(lowStockItems))
	for i := range lowStockItems {
		ptrs[i] = &lowStockItems[i]
	}
	repository.FillLent(s.db, ptrs)

	overdueLoans, err := s.lendingRepo.ListOverdue(userID, w.today)
	if err != nil {
		return nil, err
	}

	return &AlertAllResponse{
		ExpiredItems:       expiredItems,
		ExpiringSoonItems:  expiringSoonItems,
		ExpiringWarranties: warrantyEndingItems,
		OverdueLoans:       overdueLoans,
		LowStockItems:      lowStockItems,
	}, nil
}
