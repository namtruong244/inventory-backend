package models

import "time"

// Envelope Response chuẩn
type ApiResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

type ApiError struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

type ApiErrorResponse struct {
	Success bool     `json:"success"`
	Error   ApiError `json:"error"`
}

// 1. Social Auth Requests
type GoogleLoginRequest struct {
	Email     string  `json:"email" binding:"required,email"`
	Name      string  `json:"name" binding:"required"`
	AvatarURL *string `json:"avatar_url"`
}

type AppleLoginRequest struct {
	Email string `json:"email" binding:"required,email"`
	Name  string `json:"name" binding:"required"`
}

type AuthResponseData struct {
	Token string   `json:"token"`
	User  UserView `json:"user"`
}

type UserView struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	Role      string  `json:"role"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
	Provider  string  `json:"provider,omitempty"`
}

type UpdateProfileRequest struct {
	Name      *string `json:"name,omitempty"`
	Role      *string `json:"role,omitempty"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
}

// 2. Item & Dynamic Specifications
type ItemDTO struct {
	ID                 string                 `json:"id"`
	BoxID              string                 `json:"boxId"`
	CategoryID         string                 `json:"categoryId"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	Quantity           float64                `json:"quantity"`
	Unit               string                 `json:"unit"`
	MinQuantity        *float64               `json:"minQuantity,omitempty"`
	Status             string                 `json:"status"` // stored, inUse, lent, broken, disposed
	PurchasePrice      *float64               `json:"purchasePrice,omitempty"`
	PurchaseDate       *time.Time             `json:"purchaseDate,omitempty"`
	WarrantyExpiryDate *time.Time             `json:"warrantyExpiryDate,omitempty"`
	ExpiryDate         *time.Time             `json:"expiryDate,omitempty"`
	SerialNumber       *string                `json:"serialNumber,omitempty"`
	Barcode            *string                `json:"barcode,omitempty"`
	Photos             []string               `json:"photos"`
	ReceiptPhotos      []string               `json:"receiptPhotos"`
	CustomAttributes   map[string]interface{} `json:"customAttributes"` // Specifications & Attributes
	CreatedAt          time.Time              `json:"createdAt"`
	UpdatedAt          *time.Time             `json:"updatedAt,omitempty"`
}

// 3. Lending DTOs
type LendItemRequest struct {
	ItemID             string     `json:"itemId" binding:"required"`
	BorrowerName       string     `json:"borrowerName" binding:"required"`
	BorrowerContact    string     `json:"borrowerContact"`
	LentDate           time.Time  `json:"lentDate" binding:"required"`
	ExpectedReturnDate *time.Time `json:"expectedReturnDate"`
	Notes              string     `json:"notes"`
}

type LendingRecordDTO struct {
	ID                 string     `json:"id"`
	ItemID             string     `json:"itemId"`
	BorrowerName       string     `json:"borrowerName"`
	BorrowerContact    string     `json:"borrowerContact"`
	LentDate           time.Time  `json:"lentDate"`
	ExpectedReturnDate *time.Time `json:"expectedReturnDate,omitempty"`
	ActualReturnDate   *time.Time `json:"actualReturnDate,omitempty"`
	Notes              string     `json:"notes"`
}

// 4. Box DTOs
type MoveBoxRequest struct {
	NewParentID *string `json:"newParentId"`
}

type MoveItemRequest struct {
	TargetBoxID string `json:"targetBoxId" binding:"required"`
}

type UpdateQuantityRequest struct {
	Quantity float64 `json:"quantity" binding:"required,gte=0"`
}

// 5. Scan Lookup
type ScanLookupResponse struct {
	Type    string      `json:"type"` // "box" or "item"
	Payload interface{} `json:"payload"`
}
