package service_test

import (
	"testing"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/service"

	"github.com/google/uuid"
)

type mockBoxRepo struct {
	boxes map[uuid.UUID]*models.Box
}

func newMockBoxRepo() *mockBoxRepo {
	return &mockBoxRepo{boxes: make(map[uuid.UUID]*models.Box)}
}

func (m *mockBoxRepo) Create(box *models.Box) error {
	if box.ID == uuid.Nil {
		box.ID = uuid.New()
	}
	m.boxes[box.ID] = box
	return nil
}

func (m *mockBoxRepo) GetByID(id uuid.UUID, userID uuid.UUID) (*models.Box, error) {
	if b, ok := m.boxes[id]; ok && b.UserID == userID {
		return b, nil
	}
	return nil, nil
}

func (m *mockBoxRepo) GetByLabel(label string, userID *uuid.UUID) (*models.Box, error) {
	for _, b := range m.boxes {
		if b.Label == label {
			if userID == nil || b.UserID == *userID {
				return b, nil
			}
		}
	}
	return nil, nil
}

func (m *mockBoxRepo) ListByParent(userID uuid.UUID, parentID *uuid.UUID) ([]models.Box, error) {
	var result []models.Box
	for _, b := range m.boxes {
		if b.UserID == userID {
			if parentID == nil && b.ParentID == nil {
				result = append(result, *b)
			} else if parentID != nil && b.ParentID != nil && *b.ParentID == *parentID {
				result = append(result, *b)
			}
		}
	}
	return result, nil
}

func (m *mockBoxRepo) ListAll(userID uuid.UUID) ([]models.Box, error) {
	var result []models.Box
	for _, b := range m.boxes {
		if b.UserID == userID {
			result = append(result, *b)
		}
	}
	return result, nil
}

func (m *mockBoxRepo) Update(box *models.Box) error {
	m.boxes[box.ID] = box
	return nil
}

func (m *mockBoxRepo) Delete(id uuid.UUID, userID uuid.UUID, cascade bool) error {
	delete(m.boxes, id)
	return nil
}

func (m *mockBoxRepo) CountItemsInBox(boxID uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockBoxRepo) CountSubBoxesInBox(boxID uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockBoxRepo) GetBreadcrumbs(boxID uuid.UUID, userID uuid.UUID) ([]models.BreadcrumbItem, error) {
	return []models.BreadcrumbItem{}, nil
}

func (m *mockBoxRepo) IsDescendant(boxID, potentialDescendantID uuid.UUID) (bool, error) {
	return false, nil
}

type mockItemRepoForBox struct {
	repository.ItemRepository
}

func TestBoxService_ListAndNormalizeType(t *testing.T) {
	boxRepo := newMockBoxRepo()
	boxSvc := service.NewBoxService(boxRepo, nil)

	userID := uuid.New()

	// 1. Create Room (parentID == nil) with uppercase/mixed Type "Room"
	room, err := boxSvc.Create(userID, service.CreateBoxRequest{
		Name:  "Living Room",
		Type:  "Room",
		Label: "loc_living",
	})
	if err != nil {
		t.Fatalf("unexpected error creating room: %v", err)
	}
	if room.Type != "room" {
		t.Errorf("expected room.Type to be normalized to 'room', got %q", room.Type)
	}

	// 2. Create Container inside Room (parentID == room.ID)
	container, err := boxSvc.Create(userID, service.CreateBoxRequest{
		Name:     "Electronics Bin",
		Type:     "Box",
		ParentID: &room.ID,
		Label:    "loc_bin_1",
	})
	if err != nil {
		t.Fatalf("unexpected error creating container: %v", err)
	}
	if container.Type != "box" {
		t.Errorf("expected container.Type to be 'box', got %q", container.Type)
	}

	// 3. List without parentId (parentIDParam == "") -> MUST return ALL boxes (Room AND Container)
	allBoxesResult, err := boxSvc.List(userID, "", false)
	if err != nil {
		t.Fatalf("unexpected error in List: %v", err)
	}
	allBoxes, ok := allBoxesResult.([]models.Box)
	if !ok {
		t.Fatalf("expected []models.Box result, got %T", allBoxesResult)
	}
	if len(allBoxes) != 2 {
		t.Errorf("expected 2 boxes returned when parentId is empty (ListAll), got %d", len(allBoxes))
	}

	// 4. List with parentId=root -> MUST return only root boxes (Room only)
	rootBoxesResult, err := boxSvc.List(userID, "root", false)
	if err != nil {
		t.Fatalf("unexpected error in List root: %v", err)
	}
	rootBoxes := rootBoxesResult.([]models.Box)
	if len(rootBoxes) != 1 || rootBoxes[0].ID != room.ID {
		t.Errorf("expected 1 root box (the Room), got %d", len(rootBoxes))
	}

	// 5. List with parentId=<room.ID> -> MUST return only children of Room (Container only)
	subBoxesResult, err := boxSvc.List(userID, room.ID.String(), false)
	if err != nil {
		t.Fatalf("unexpected error in List by room ID: %v", err)
	}
	subBoxes := subBoxesResult.([]models.Box)
	if len(subBoxes) != 1 || subBoxes[0].ID != container.ID {
		t.Errorf("expected 1 subBox in room, got %d", len(subBoxes))
	}
}
