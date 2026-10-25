package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LendingRecord struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"userId"`
	ItemID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"itemId"`
	BorrowerName       string     `gorm:"size:150;not null" json:"borrowerName"`
	BorrowerContact    string     `gorm:"size:150;default:''" json:"borrowerContact"`
	LentDate           time.Time  `gorm:"default:now()" json:"lentDate"`
	ExpectedReturnDate *time.Time `json:"expectedReturnDate"`
	ActualReturnDate   *time.Time `json:"actualReturnDate"`
	Notes              string     `gorm:"type:text;default:''" json:"notes"`
	// Quantity is how many units were lent. IsPartial loans took units out of
	// items.quantity (put back on return); whole loans lend the item itself.
	Quantity  float64 `gorm:"type:numeric(12,2);default:0;not null" json:"quantity"`
	IsPartial bool    `gorm:"default:false;not null" json:"isPartial"`
	// PreviousStatus is the item status before lending; restored on return.
	PreviousStatus string    `gorm:"size:30;default:''" json:"-"`
	CreatedAt      time.Time `json:"createdAt"`

	// Virtual fields for responses
	ItemName string `gorm:"-" json:"itemName,omitempty"`
	ItemCode string `gorm:"-" json:"itemCode,omitempty"`
}

func (l *LendingRecord) BeforeCreate(tx *gorm.DB) (err error) {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	if l.LentDate.IsZero() {
		l.LentDate = time.Now()
	}
	return
}
