package service_test

import (
	"testing"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/models"
	"inventory_backend/internal/service"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// treeBoxRepo adds what Move needs from a real repository on top of mockBoxRepo:
// a working IsDescendant and "not found" as an error.
type treeBoxRepo struct{ *mockBoxRepo }

func (r treeBoxRepo) GetByID(id, userID uuid.UUID) (*models.Box, error) {
	if b, _ := r.mockBoxRepo.GetByID(id, userID); b != nil {
		return b, nil
	}
	return nil, gorm.ErrRecordNotFound
}

// IsDescendant reports whether candidate sits below boxID, by walking up its parents.
func (r treeBoxRepo) IsDescendant(boxID, candidate, userID uuid.UUID) (bool, error) {
	for cur := &candidate; cur != nil; {
		b := r.boxes[*cur]
		if b == nil {
			return false, nil
		}
		if b.ParentID != nil && *b.ParentID == boxID {
			return true, nil
		}
		cur = b.ParentID
	}
	return false, nil
}

func TestBoxService_MoveRejectsCycles(t *testing.T) {
	user := uuid.New()
	repo := treeBoxRepo{newMockBoxRepo()}
	mk := func(name string, parent *uuid.UUID) *models.Box {
		b := &models.Box{ID: uuid.New(), UserID: user, Name: name, Label: name, ParentID: parent}
		repo.boxes[b.ID] = b
		return b
	}
	root := mk("root", nil)
	child := mk("child", &root.ID)
	grandchild := mk("grandchild", &child.ID)
	elsewhere := mk("elsewhere", nil)
	svc := service.NewBoxService(repo, nil)

	invalid := func(err error) bool {
		e, ok := apperr.As(err)
		return ok && e.Kind == apperr.KindInvalid
	}

	if err := svc.Move(user, root.ID, &root.ID); !invalid(err) {
		t.Errorf("a box inside itself must be rejected, got %v", err)
	}
	if err := svc.Move(user, root.ID, &child.ID); !invalid(err) {
		t.Errorf("a box inside its child must be rejected, got %v", err)
	}
	if err := svc.Move(user, root.ID, &grandchild.ID); !invalid(err) {
		t.Errorf("a box inside a deeper descendant must be rejected, got %v", err)
	}
	if root.ParentID != nil {
		t.Errorf("a rejected move must leave the tree unchanged")
	}

	if err := svc.Move(user, grandchild.ID, &elsewhere.ID); err != nil {
		t.Fatalf("moving to an unrelated box: %v", err)
	}
	if grandchild.ParentID == nil || *grandchild.ParentID != elsewhere.ID {
		t.Errorf("grandchild should now sit under elsewhere")
	}
	// Once moved out, the old parent chain is no longer a cycle risk.
	if err := svc.Move(user, child.ID, &grandchild.ID); err != nil {
		t.Errorf("child under its former grandchild is legal after that move: %v", err)
	}
	if err := svc.Move(user, child.ID, nil); err != nil || child.ParentID != nil {
		t.Errorf("a nil parent moves the box to the root: err=%v parent=%v", err, child.ParentID)
	}

	stranger := uuid.New()
	if err := svc.Move(stranger, root.ID, nil); err == nil {
		t.Errorf("another user must not move this box")
	}
	if err := svc.Move(user, root.ID, ptrUUID(uuid.New())); err == nil {
		t.Errorf("a missing target parent must be rejected")
	}
}

func ptrUUID(u uuid.UUID) *uuid.UUID { return &u }
