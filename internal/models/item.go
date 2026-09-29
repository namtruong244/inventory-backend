package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Item struct {
	ID                 uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	UserID             uuid.UUID              `gorm:"type:uuid;not null;index" json:"userId"`
	BoxID              uuid.UUID              `gorm:"type:uuid;not null;index" json:"boxId"`
	CategoryID         string                 `gorm:"size:100;not null;index" json:"categoryId"`
	Name               string                 `gorm:"size:255;not null" json:"name"`
	Description        string                 `gorm:"type:text;default:''" json:"description"`
	Quantity           float64                `gorm:"type:numeric(12,2);default:1.0;not null" json:"quantity"`
	Unit               string                 `gorm:"size:30;default:'pcs';not null" json:"unit"`
	MinQuantity        *float64               `gorm:"type:numeric(12,2)" json:"minQuantity"`
	Status             string                 `gorm:"size:30;default:'stored'" json:"status"`
	PurchasePrice      *float64               `gorm:"type:numeric(15,2)" json:"purchasePrice"`
	PurchaseDate       *string                `gorm:"type:varchar(30)" json:"purchaseDate"`
	SerialNumber       *string                `gorm:"size:150" json:"serialNumber"`
	Barcode            *string                `gorm:"size:150;index" json:"barcode"`
	WarrantyExpiryDate *string                `gorm:"type:varchar(30);index" json:"warrantyExpiryDate"`
	ExpiryDate         *string                `gorm:"type:varchar(30);index" json:"expiryDate"`
	Photos             []string               `gorm:"type:jsonb;serializer:json;default:'[]'" json:"photos"`
	ReceiptPhotos      []string               `gorm:"type:jsonb;serializer:json;default:'[]'" json:"receiptPhotos"`
	CustomAttributes   map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"customAttributes"`
	CreatedAt          time.Time              `json:"createdAt"`
	UpdatedAt          time.Time              `json:"updatedAt"`

	// Virtual fields for response
	IsLowStock   bool   `gorm:"-" json:"isLowStock"`
	BoxName      string `gorm:"-" json:"boxName,omitempty"`
	CategoryName string `gorm:"-" json:"categoryName,omitempty"`
}

func (i *Item) ComputeIsLowStock() bool {
	if i.MinQuantity != nil && i.Quantity <= *i.MinQuantity {
		return true
	}
	return false
}

func (i *Item) BeforeCreate(tx *gorm.DB) (err error) {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	if i.Photos == nil {
		i.Photos = make([]string, 0)
	}
	if i.ReceiptPhotos == nil {
		i.ReceiptPhotos = make([]string, 0)
	}
	if i.CustomAttributes == nil {
		i.CustomAttributes = make(map[string]interface{})
	}
	return
}

func (i *Item) AfterFind(tx *gorm.DB) (err error) {
	i.IsLowStock = i.ComputeIsLowStock()
	if i.Photos == nil {
		i.Photos = make([]string, 0)
	}
	if i.ReceiptPhotos == nil {
		i.ReceiptPhotos = make([]string, 0)
	}
	if i.CustomAttributes == nil {
		i.CustomAttributes = make(map[string]interface{})
	}
	return
}
