package service_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"testing"

	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/service"

	"github.com/google/uuid"
)

type mockItemRepoForExport struct {
	repository.ItemRepository
	items []models.Item
}

func (m *mockItemRepoForExport) List(userID uuid.UUID, f repository.ItemFilter) ([]models.Item, int64, error) {
	start := (f.Page - 1) * f.Limit
	if start >= len(m.items) {
		return nil, int64(len(m.items)), nil
	}
	end := start + f.Limit
	if end > len(m.items) {
		end = len(m.items)
	}
	return m.items[start:end], int64(len(m.items)), nil
}

func newExportService(n int) service.ReportService {
	repo := &mockItemRepoForExport{}
	for i := 0; i < n; i++ {
		repo.items = append(repo.items, models.Item{ID: uuid.New(), Name: fmt.Sprintf("item-%d", i), Status: "stored"})
	}
	return service.NewReportService(nil, repo, nil, nil)
}

func TestReportService_ExportCSVAllBatches(t *testing.T) {
	// 1200 items spans three batches and must not be truncated.
	svc := newExportService(1200)
	var buf bytes.Buffer
	if err := svc.ExportItems(uuid.New(), "csv", &buf); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1201 {
		t.Errorf("expected header + 1200 rows, got %d", len(rows))
	}
}

func TestReportService_ExportCSVNeutralizesFormulas(t *testing.T) {
	repo := &mockItemRepoForExport{items: []models.Item{{ID: uuid.New(), Name: "=HYPERLINK(\"x\")", Status: "stored"}}}
	svc := service.NewReportService(nil, repo, nil, nil)
	var buf bytes.Buffer
	if err := svc.ExportItems(uuid.New(), "csv", &buf); err != nil {
		t.Fatal(err)
	}
	rows, _ := csv.NewReader(&buf).ReadAll()
	if rows[1][1][0] != '\'' {
		t.Errorf("expected formula to be prefixed, got %q", rows[1][1])
	}
}

func TestReportService_ExportJSON(t *testing.T) {
	for _, n := range []int{0, 1, 501} {
		var buf bytes.Buffer
		if err := newExportService(n).ExportItems(uuid.New(), "json", &buf); err != nil {
			t.Fatal(err)
		}
		var out []map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			t.Fatalf("n=%d invalid json: %v", n, err)
		}
		if len(out) != n {
			t.Errorf("n=%d got %d", n, len(out))
		}
	}
}
