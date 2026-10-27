package service_test

import (
	"errors"
	"testing"
	"time"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/service"

	"github.com/google/uuid"
)

type mockItemRepo struct {
	repository.ItemRepository
	items map[uuid.UUID]*models.Item
	moves *memMovementRepo
}

func (m *mockItemRepo) Create(i *models.Item) error { i.ID = uuid.New(); m.items[i.ID] = i; return nil }
func (m *mockItemRepo) GetByID(id, userID uuid.UUID) (*models.Item, error) {
	if i, ok := m.items[id]; ok && i.UserID == userID {
		cp := *i
		return &cp, nil
	}
	return nil, errors.New("record not found")
}
func (m *mockItemRepo) Update(i *models.Item) error { m.items[i.ID] = i; return nil }
func (m *mockItemRepo) ChangeQuantity(id, userID uuid.UUID, c repository.QuantityChange) (*models.Item, float64, error) {
	it, ok := m.items[id]
	if !ok || it.UserID != userID {
		return nil, 0, apperr.NotFound("item not found")
	}
	next := it.Quantity
	if c.Delta != nil {
		next += *c.Delta
	} else if c.Set != nil {
		next = *c.Set
	}
	if next < 0 {
		return nil, 0, apperr.Invalid("quantity cannot be negative")
	}
	applied := next - it.Quantity
	it.Quantity = next
	if applied != 0 && m.moves != nil {
		_ = m.moves.Create(&models.StockMovement{UserID: userID, ItemID: id, Delta: applied, QuantityAfter: next, Reason: c.Reason, Note: c.Note})
	}
	cp := *it
	return &cp, applied, nil
}

type memMovementRepo struct{ list []models.StockMovement }

func (m *memMovementRepo) Create(mv *models.StockMovement) error {
	mv.CreatedAt = time.Now().UTC()
	m.list = append([]models.StockMovement{*mv}, m.list...)
	return nil
}
func (m *memMovementRepo) ListByItem(_, _ uuid.UUID, _ int) ([]models.StockMovement, error) {
	return m.list, nil
}

func setupItemSvc(t *testing.T) (service.ItemService, uuid.UUID, uuid.UUID) {
	t.Helper()
	userID := uuid.New()
	boxes := newMockBoxRepo()
	box := &models.Box{ID: uuid.New(), UserID: userID, Name: "b", Label: "b"}
	boxes.boxes[box.ID] = box
	moves := &memMovementRepo{}
	svc := service.NewItemService(&mockItemRepo{items: map[uuid.UUID]*models.Item{}, moves: moves}, boxes, nil, nil, anyCategoryRepo{}, moves)
	return svc, userID, box.ID
}

func isInvalid(err error) bool {
	e, ok := apperr.As(err)
	return ok && e.Kind == apperr.KindInvalid
}

func ptr[T any](v T) *T { return &v }

func TestItemService_CreateValidation(t *testing.T) {
	svc, userID, boxID := setupItemSvc(t)
	base := func() service.CreateItemRequest {
		return service.CreateItemRequest{Name: "n", BoxID: boxID, CategoryID: "c", Unit: ptr("pcs")}
	}

	cases := map[string]func(*service.CreateItemRequest){
		"negative quantity": func(r *service.CreateItemRequest) { r.Quantity = ptr(-1.0) },
		"unknown status":    func(r *service.CreateItemRequest) { r.Status = ptr("weird") },
		"lent status":       func(r *service.CreateItemRequest) { r.Status = ptr("lent") },
		"bad date":          func(r *service.CreateItemRequest) { r.ExpiryDate = ptr("not-a-date") },
	}
	for name, mutate := range cases {
		req := base()
		mutate(&req)
		if _, err := svc.Create(userID, req); !isInvalid(err) {
			t.Errorf("%s: expected invalid error, got %v", name, err)
		}
	}

	req := base()
	req.ExpiryDate = ptr("2026-12-31T23:59:00+07:00")
	item, err := svc.Create(userID, req)
	if err != nil {
		t.Fatal(err)
	}
	if item.ExpiryDate == nil || *item.ExpiryDate != "2026-12-31" {
		t.Errorf("expiry date not normalized: %v", item.ExpiryDate)
	}
}

func TestItemService_UpdateStatusAndMinQuantity(t *testing.T) {
	svc, userID, boxID := setupItemSvc(t)
	item, err := svc.Create(userID, service.CreateItemRequest{
		Name: "n", BoxID: boxID, CategoryID: "c", Unit: ptr("pcs"), MinQuantity: ptr(3.0),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Update(userID, item.ID, service.UpdateItemRequest{Status: ptr("lent")}); !isInvalid(err) {
		t.Errorf("setting lent via update must be rejected, got %v", err)
	}
	if _, err := svc.Update(userID, item.ID, service.UpdateItemRequest{Quantity: ptr(-2.0)}); !isInvalid(err) {
		t.Errorf("negative quantity must be rejected, got %v", err)
	}

	updated, err := svc.Update(userID, item.ID, service.UpdateItemRequest{ClearMinQuantity: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.MinQuantity != nil {
		t.Errorf("minQuantity should be cleared")
	}
}

func TestBoxService_RejectsUnknownType(t *testing.T) {
	svc := service.NewBoxService(newMockBoxRepo(), nil)
	_, err := svc.Create(uuid.New(), service.CreateBoxRequest{Name: "x", Type: "spaceship", Label: "L"})
	if !isInvalid(err) {
		t.Errorf("expected invalid error for unknown box type, got %v", err)
	}
}

// anyCategoryRepo resolves every category value to itself.
type anyCategoryRepo struct{}

func (anyCategoryRepo) List(uuid.UUID) ([]models.Category, error) { return nil, nil }
func (anyCategoryRepo) Create(*models.Category) error             { return nil }
func (anyCategoryRepo) DeleteByName(uuid.UUID, string) error      { return nil }
func (anyCategoryRepo) GetByName(_ uuid.UUID, n string) (*models.Category, error) {
	return &models.Category{Name: n}, nil
}
func (anyCategoryRepo) Resolve(_ uuid.UUID, v string) (*models.Category, error) {
	return &models.Category{Name: v}, nil
}

func TestItemService_UpdateQuantityRecordsMovement(t *testing.T) {
	svc, userID, boxID := setupItemSvc(t)
	item, err := svc.Create(userID, service.CreateItemRequest{Name: "n", BoxID: boxID, CategoryID: "c", Unit: ptr("pcs"), Quantity: ptr(10.0)})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateQuantity(userID, item.ID, 4, "consume", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateQuantity(userID, item.ID, 4, "bogus", ""); !isInvalid(err) {
		t.Fatalf("want invalid reason, got %v", err)
	}
	h, err := svc.Movements(userID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Movements) != 2 || h.Movements[0].Delta != -6 || h.Movements[0].QuantityAfter != 4 {
		t.Fatalf("unexpected movements: %+v", h.Movements)
	}
	if h.EstimatedDaysLeft == nil || h.AvgDailyConsumption30d != 6.0/30 {
		t.Fatalf("unexpected estimate: %+v", h)
	}
	if _, err := svc.Movements(uuid.New(), item.ID); err == nil {
		t.Fatal("other user must not read movements")
	}
}

func TestItemService_PhotoOwnership(t *testing.T) {
	svc, userID, boxID := setupItemSvc(t)
	other := uuid.New()
	mine := "https://cdn.example/uploads/items/" + userID.String() + "/a.jpg"
	theirs := "https://cdn.example/uploads/items/" + other.String() + "/b.jpg"
	legacy := "https://cdn.example/uploads/items/c0ffee00-0000-4000-8000-000000000000.jpg"
	external := "https://images.unsplash.com/photo-1?w=800"

	req := service.CreateItemRequest{Name: "n", BoxID: boxID, CategoryID: "c", Unit: ptr("pcs")}

	bad := req
	bad.Photos = []string{mine, theirs}
	if _, err := svc.Create(userID, bad); !isInvalid(err) {
		t.Errorf("another user's file must be rejected on create, got %v", err)
	}
	badReceipt := req
	badReceipt.ReceiptPhotos = []string{theirs}
	if _, err := svc.Create(userID, badReceipt); !isInvalid(err) {
		t.Errorf("another user's receipt must be rejected on create, got %v", err)
	}

	ok := req
	ok.Photos = []string{mine, legacy, external}
	item, err := svc.Create(userID, ok)
	if err != nil {
		t.Fatalf("own, legacy and external photos are fine: %v", err)
	}

	if _, err := svc.Update(userID, item.ID, service.UpdateItemRequest{Photos: []string{mine, theirs}}); !isInvalid(err) {
		t.Errorf("attaching another user's file on update must be rejected, got %v", err)
	}
	if _, err := svc.Update(userID, item.ID, service.UpdateItemRequest{Photos: []string{legacy, mine, external, mine}}); err != nil {
		t.Errorf("re-sending the item's own photos must work: %v", err)
	}
}
