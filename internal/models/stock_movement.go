package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StockMovement is an append-only audit record of a quantity or location change.
type StockMovement struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index" json:"userId"`
	ItemID        uuid.UUID `gorm:"type:uuid;not null;index:idx_movements_item_created,priority:1" json:"itemId"`
	Delta         float64   `gorm:"not null;default:0" json:"delta"`
	QuantityAfter float64   `gorm:"not null;default:0" json:"quantityAfter"`
	// Reason is one of the MovementReason* values.
	Reason    string    `gorm:"size:30;not null" json:"reason"`
	Note      string    `gorm:"type:text;default:''" json:"note"`
	CreatedAt time.Time `gorm:"index:idx_movements_item_created,priority:2,sort:desc" json:"createdAt"`
}

const (
	MovementInitial    = "initial"
	MovementRestock    = "restock"
	MovementConsume    = "consume"
	MovementDamaged    = "damaged"
	MovementLost       = "lost"
	MovementCorrection = "correction"
	MovementAdjust     = "adjust"
	MovementMove       = "move"
)

// ValidMovementReasons are the reasons a client may give when changing a quantity.
var ValidMovementReasons = map[string]bool{
	MovementRestock: true, MovementConsume: true, MovementDamaged: true,
	MovementLost: true, MovementCorrection: true, MovementAdjust: true,
}

func (m *StockMovement) BeforeCreate(tx *gorm.DB) (err error) {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return
}
