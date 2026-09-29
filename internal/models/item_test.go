package models_test

import (
	"testing"

	"inventory_backend/internal/models"
)

func TestItemLowStockComputation(t *testing.T) {
	// Case 1: MinQuantity is nil -> IsLowStock must be false
	item1 := models.Item{
		Quantity:    0,
		MinQuantity: nil,
	}
	if item1.ComputeIsLowStock() {
		t.Errorf("expected IsLowStock=false when MinQuantity is nil, got true")
	}

	// Case 2: Quantity > MinQuantity -> IsLowStock must be false
	minQty := 3.0
	item2 := models.Item{
		Quantity:    5.0,
		MinQuantity: &minQty,
	}
	if item2.ComputeIsLowStock() {
		t.Errorf("expected IsLowStock=false when Quantity > MinQuantity, got true")
	}

	// Case 3: Quantity == MinQuantity -> IsLowStock must be true
	item3 := models.Item{
		Quantity:    3.0,
		MinQuantity: &minQty,
	}
	if !item3.ComputeIsLowStock() {
		t.Errorf("expected IsLowStock=true when Quantity == MinQuantity, got false")
	}

	// Case 4: Quantity < MinQuantity -> IsLowStock must be true
	item4 := models.Item{
		Quantity:    1.0,
		MinQuantity: &minQty,
	}
	if !item4.ComputeIsLowStock() {
		t.Errorf("expected IsLowStock=true when Quantity < MinQuantity, got false")
	}
}
