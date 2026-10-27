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
	"github.com/jackc/pgx/v5/pgconn"
)

// lendRepo scripts the atomic repository calls; the SQL itself is covered by the
// integration tests.
type lendRepo struct {
	repository.LendingRepository
	existing  *models.LendingRecord
	lendErr   error
	returnErr error
	lent      []*models.LendingRecord
}

func (r *lendRepo) GetByID(id, userID uuid.UUID) (*models.LendingRecord, error) {
	if r.existing != nil && r.existing.ID == id && r.existing.UserID == userID {
		return r.existing, nil
	}
	return nil, errors.New("not found")
}

func (r *lendRepo) LendItem(userID uuid.UUID, rec *models.LendingRecord) (*models.Item, error) {
	if r.lendErr != nil {
		return nil, r.lendErr
	}
	r.lent = append(r.lent, rec)
	code := "BC-1"
	return &models.Item{ID: rec.ItemID, Name: "Drill", Barcode: &code}, nil
}

func (r *lendRepo) ReturnLoan(uuid.UUID, uuid.UUID) error { return r.returnErr }

func kindOf(err error) apperr.Kind {
	if e, ok := apperr.As(err); ok {
		return e.Kind
	}
	return 0
}

func TestLendingService_LendItem(t *testing.T) {
	user, item := uuid.New(), uuid.New()
	day := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	req := func() service.LendItemRequest {
		return service.LendItemRequest{ItemID: item, BorrowerName: "Bob", LentDate: &day}
	}
	ptrTime := func(d time.Time) *time.Time { return &d }

	t.Run("lends and fills the item info", func(t *testing.T) {
		repo := &lendRepo{}
		rec, err := service.NewLendingService(repo, nil, nil).LendItem(user, req())
		if err != nil {
			t.Fatal(err)
		}
		if rec.ItemName != "Drill" || rec.ItemCode != "BC-1" || rec.UserID != user {
			t.Errorf("record = %+v", rec)
		}
	})

	t.Run("a replayed lend with the same id returns the stored record", func(t *testing.T) {
		id := uuid.New()
		repo := &lendRepo{existing: &models.LendingRecord{ID: id, UserID: user, ItemID: item}}
		r := req()
		r.ID = &id
		rec, err := service.NewLendingService(repo, nil, nil).LendItem(user, r)
		if err != nil || rec != repo.existing {
			t.Fatalf("rec=%v err=%v", rec, err)
		}
		if len(repo.lent) != 0 {
			t.Errorf("a replay must not lend again")
		}
	})

	t.Run("validation", func(t *testing.T) {
		svc := service.NewLendingService(&lendRepo{}, nil, nil)
		before := req()
		before.ExpectedReturnDate = ptrTime(day.AddDate(0, 0, -1))
		if _, err := svc.LendItem(user, before); kindOf(err) != apperr.KindInvalid {
			t.Errorf("return date before lent date: %v", err)
		}
		sameDay := req()
		sameDay.ExpectedReturnDate = ptrTime(day)
		if _, err := svc.LendItem(user, sameDay); err != nil {
			t.Errorf("returning on the lent day is allowed: %v", err)
		}
		for _, q := range []float64{0, -2} {
			bad := req()
			bad.Quantity = &q
			if _, err := svc.LendItem(user, bad); kindOf(err) != apperr.KindInvalid {
				t.Errorf("quantity %v: %v", q, err)
			}
		}
	})

	t.Run("repository errors become typed errors", func(t *testing.T) {
		cases := []struct {
			err  error
			want apperr.Kind
		}{
			{repository.ErrItemNotFound, apperr.KindNotFound},
			{repository.ErrItemAlreadyLent, apperr.KindConflict},
			{repository.ErrItemNotLendable, apperr.KindInvalid},
			{repository.ErrQuantityExceeds, apperr.KindInvalid},
			{&pgconn.PgError{Code: "23505"}, apperr.KindConflict},
		}
		for _, tc := range cases {
			_, err := service.NewLendingService(&lendRepo{lendErr: tc.err}, nil, nil).LendItem(user, req())
			if kindOf(err) != tc.want {
				t.Errorf("%v -> kind %v, want %v", tc.err, kindOf(err), tc.want)
			}
		}
		// An unexpected failure must not be dressed up as a business error.
		_, err := service.NewLendingService(&lendRepo{lendErr: errors.New("db down")}, nil, nil).LendItem(user, req())
		if err == nil || kindOf(err) != 0 {
			t.Errorf("unexpected errors stay untyped, got %v", err)
		}
	})
}

func TestLendingService_ReturnItem(t *testing.T) {
	user, id := uuid.New(), uuid.New()
	cases := []struct {
		err  error
		want apperr.Kind
	}{
		{nil, 0},
		{repository.ErrLoanNotFound, apperr.KindNotFound},
		{repository.ErrLoanAlreadyClosed, apperr.KindConflict},
	}
	for _, tc := range cases {
		err := service.NewLendingService(&lendRepo{returnErr: tc.err}, nil, nil).ReturnItem(user, id)
		if (tc.err == nil) != (err == nil) || kindOf(err) != tc.want {
			t.Errorf("return err %v -> %v (kind %v), want kind %v", tc.err, err, kindOf(err), tc.want)
		}
	}
}
