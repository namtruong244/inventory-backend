package service_test

import (
	"context"
	"io"
	"testing"
	"time"

	"inventory_backend/internal/models"
	"inventory_backend/internal/service"

	"github.com/google/uuid"
)

type fakeTrashRepo struct {
	users   []uuid.UUID
	media   map[uuid.UUID][]string
	emptied []uuid.UUID
	cutoffs []time.Time
}

func (f *fakeTrashRepo) List(uuid.UUID) ([]models.Item, []models.Box, error) { return nil, nil, nil }
func (f *fakeTrashRepo) RestoreItem(_, _ uuid.UUID) error                    { return nil }
func (f *fakeTrashRepo) RestoreBox(_, _ uuid.UUID) error                     { return nil }
func (f *fakeTrashRepo) PurgeItem(_, _ uuid.UUID) error                      { return nil }
func (f *fakeTrashRepo) PurgeBox(_, _ uuid.UUID) error                       { return nil }
func (f *fakeTrashRepo) Empty(u uuid.UUID, c time.Time) (int64, error) {
	f.emptied = append(f.emptied, u)
	f.cutoffs = append(f.cutoffs, c)
	return 2, nil
}
func (f *fakeTrashRepo) UsersWithTrashBefore(time.Time) ([]uuid.UUID, error) { return f.users, nil }
func (f *fakeTrashRepo) MediaTrashedBefore(u uuid.UUID, _ time.Time) ([]string, error) {
	return f.media[u], nil
}

type fakeFiles struct{ deleted []string }

func (f *fakeFiles) Upload(context.Context, io.Reader, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeFiles) Delete(_ context.Context, u string) error {
	f.deleted = append(f.deleted, u)
	return nil
}

func TestTrashService_PurgeExpired(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	repo := &fakeTrashRepo{users: []uuid.UUID{a, b}, media: map[uuid.UUID][]string{a: {"/uploads/x.jpg"}}}
	files := &fakeFiles{}
	svc := service.NewTrashService(repo, service.WithTrashFileStorage(files))

	n, err := svc.PurgeExpired(30)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 || len(repo.emptied) != 2 {
		t.Fatalf("removed=%d emptied=%d", n, len(repo.emptied))
	}
	want := time.Now().UTC().AddDate(0, 0, -30)
	if d := repo.cutoffs[0].Sub(want); d > time.Minute || d < -time.Minute {
		t.Fatalf("cutoff %v not ~30 days ago", repo.cutoffs[0])
	}
	if len(files.deleted) != 1 || files.deleted[0] != "/uploads/x.jpg" {
		t.Fatalf("deleted files = %v", files.deleted)
	}
	if _, err := svc.PurgeExpired(0); err == nil {
		t.Fatal("0 days would purge everything and must be rejected")
	}
}
