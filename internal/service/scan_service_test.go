package service_test

import (
	"testing"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/service"

	"github.com/google/uuid"
)

// scanItemRepo answers GetByCode only, scoped to the asking user like the real repository.
type scanItemRepo struct {
	repository.ItemRepository
	items []*models.Item
}

func (r *scanItemRepo) GetByCode(code string, userID *uuid.UUID) (*models.Item, error) {
	for _, it := range r.items {
		if userID != nil && it.UserID != *userID {
			continue
		}
		if (it.Barcode != nil && *it.Barcode == code) || it.ID.String() == code {
			return it, nil
		}
	}
	return nil, apperr.NotFound("no item")
}

func scanType(t *testing.T, svc service.ScanService, user uuid.UUID, code string) string {
	t.Helper()
	res, err := svc.Lookup(user, code)
	if err != nil {
		return "error:" + err.Error()
	}
	return res.Type
}

func TestScanService_Lookup(t *testing.T) {
	user, other := uuid.New(), uuid.New()
	boxes := newMockBoxRepo()
	box := &models.Box{ID: uuid.New(), UserID: user, Name: "Garage", Label: "BOX-1", Type: "room"}
	foreign := &models.Box{ID: uuid.New(), UserID: other, Name: "Theirs", Label: "BOX-X"}
	boxes.boxes[box.ID], boxes.boxes[foreign.ID] = box, foreign

	barcode := "5901234123457"
	// An item whose barcode equals a box label: the box must win.
	clash := "BOX-1"
	item := &models.Item{ID: uuid.New(), UserID: user, Name: "Drill", Barcode: &barcode}
	clashItem := &models.Item{ID: uuid.New(), UserID: user, Name: "Clash", Barcode: &clash}
	svc := service.NewScanService(boxes, &scanItemRepo{items: []*models.Item{item, clashItem}})

	cases := []struct {
		name, code, want string
	}{
		{"box label", "BOX-1", "box"},
		{"label wins over an item barcode", "BOX-1", "box"},
		{"surrounding whitespace is ignored", " \t5901234123457\r\n", "item"},
		{"padded label", "BOX-1\n", "box"},
		{"item id", item.ID.String(), "item"},
		{"box id from a QR code", box.ID.String(), "box"},
		{"another user's box id", foreign.ID.String(), "error:no matching box or item found for the provided code"},
		{"another user's label", "BOX-X", "error:no matching box or item found for the provided code"},
		{"unknown code", "nope", "error:no matching box or item found for the provided code"},
		{"blank code", "  ", "error:code parameter is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scanType(t, svc, user, tc.code); got != tc.want {
				t.Errorf("Lookup(%q) = %q, want %q", tc.code, got, tc.want)
			}
		})
	}

	if _, err := svc.Lookup(user, "nope"); err != nil {
		if e, ok := apperr.As(err); !ok || e.Kind != apperr.KindNotFound {
			t.Errorf("unknown code must be a NotFound error, got %v", err)
		}
	}
}
