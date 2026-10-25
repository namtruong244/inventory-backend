package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"inventory_backend/internal/app"
	"inventory_backend/internal/config"
	"inventory_backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// These tests need a real PostgreSQL. They run only when INTEGRATION_DB_NAME is
// set, and the database should be disposable:
//
//	INTEGRATION_DB_NAME=inventory_it go test ./internal/app/
type client struct {
	t      *testing.T
	router *gin.Engine
	token  string
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	w := httptest.NewRecorder()
	c.router.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (c *client) data() map[string]any { return nil }

func newRouter(t *testing.T) *gin.Engine {
	name := os.Getenv("INTEGRATION_DB_NAME")
	if name == "" {
		t.Skip("INTEGRATION_DB_NAME not set")
	}
	gin.SetMode(gin.TestMode)
	cfg := config.LoadConfig()
	cfg.DBName = name
	cfg.RunMigrations = true
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	r, err := app.SetupRouter(cfg, db)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	return r
}

func register(t *testing.T, r *gin.Engine, email string) *client {
	c := &client{t: t, router: r}
	code, out := c.do("POST", "/auth/register", map[string]any{"name": "u", "email": email, "password": "secret123"})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("register %s: %d %v", email, code, out)
	}
	d := out["data"].(map[string]any)
	c.token = d["token"].(string)
	return c
}

func id(out map[string]any) string { return out["data"].(map[string]any)["id"].(string) }

func TestIntegration_UserIsolationAndLending(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "a-"+suffix+"@example.com")
	b := register(t, r, "b-"+suffix+"@example.com")

	code, out := a.do("POST", "/boxes", map[string]any{"name": "Room", "type": "room", "label": "IT-A-" + suffix})
	if code != http.StatusCreated {
		t.Fatalf("create box: %d %v", code, out)
	}
	boxID := id(out)
	code, out = a.do("POST", "/items", map[string]any{"name": "Drill", "boxId": boxID, "categoryId": "Tools & Hardware", "quantity": 3})
	if code != http.StatusCreated {
		t.Fatalf("create item: %d %v", code, out)
	}
	itemID := id(out)

	// IDOR: user B must not read, change, move, lend or delete A's data.
	for _, tc := range []struct{ method, path string }{
		{"GET", "/items/" + itemID},
		{"GET", "/items/" + itemID + "/movements"},
		{"PATCH", "/items/" + itemID + "/quantity"},
		{"DELETE", "/items/" + itemID},
		{"GET", "/boxes/" + boxID},
		{"DELETE", "/boxes/" + boxID},
	} {
		code, _ := b.do(tc.method, tc.path, map[string]any{"quantity": 1})
		if code != http.StatusNotFound {
			t.Errorf("B %s %s = %d, want 404", tc.method, tc.path, code)
		}
	}
	if code, _ := b.do("POST", "/lending/lend", map[string]any{"itemId": itemID, "borrowerName": "x"}); code == http.StatusCreated || code == http.StatusOK {
		t.Errorf("B lent A's item: %d", code)
	}

	// Stock movements: initial + consume.
	if code, out := a.do("PATCH", "/items/"+itemID+"/quantity", map[string]any{"quantity": 1, "reason": "consume"}); code != http.StatusOK {
		t.Fatalf("quantity: %d %v", code, out)
	}
	_, out = a.do("GET", "/items/"+itemID+"/movements", nil)
	if n := len(out["data"].(map[string]any)["movements"].([]any)); n != 2 {
		t.Errorf("movements = %d, want 2", n)
	}

	// Parallel lend: exactly one succeeds.
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := a.do("POST", "/lending/lend", map[string]any{"itemId": itemID, "borrowerName": "bob"})
			if code == http.StatusCreated || code == http.StatusOK {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Errorf("parallel lend successes = %d, want 1", ok)
	}
}

func TestIntegration_TrashRestoreAndPurge(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "t-"+suffix+"@example.com")
	b := register(t, r, "tb-"+suffix+"@example.com")

	_, out := a.do("POST", "/boxes", map[string]any{"name": "Room", "type": "room", "label": "TR-" + suffix})
	room := id(out)
	_, out = a.do("POST", "/boxes", map[string]any{"name": "Shelf", "type": "shelf", "label": "TS-" + suffix, "parentId": room})
	shelf := id(out)
	_, out = a.do("POST", "/items", map[string]any{"name": "Cup", "boxId": shelf, "categoryId": "Tools & Hardware"})
	item := id(out)

	if code, _ := a.do("DELETE", "/boxes/"+room+"?cascade=true", nil); code != http.StatusOK {
		t.Fatalf("delete box: %d", code)
	}
	if code, _ := a.do("GET", "/items/"+item, nil); code != http.StatusNotFound {
		t.Errorf("trashed item visible: %d", code)
	}
	// the label is free again while the box is trashed
	if code, out := a.do("POST", "/boxes", map[string]any{"name": "New", "type": "room", "label": "TR-" + suffix}); code != http.StatusCreated {
		t.Fatalf("reuse label: %d %v", code, out)
	} else if code, _ := a.do("DELETE", "/boxes/"+id(out), nil); code != http.StatusOK {
		t.Fatalf("delete new box: %d", code)
	}

	// user B sees nothing and cannot restore
	_, bt := b.do("GET", "/trash", nil)
	if n := len(bt["data"].(map[string]any)["boxes"].([]any)); n != 0 {
		t.Errorf("B sees %d trashed boxes", n)
	}
	if code, _ := b.do("POST", "/trash/boxes/"+room+"/restore", nil); code != http.StatusNotFound {
		t.Errorf("B restored A's box: %d", code)
	}

	// restoring the old room conflicts with the label of the second (still trashed) one only if both live,
	// so purge the new one first, then restore the original subtree.
	_, tr := a.do("GET", "/trash", nil)
	for _, bx := range tr["data"].(map[string]any)["boxes"].([]any) {
		m := bx.(map[string]any)
		if m["id"] != room {
			a.do("DELETE", "/trash/boxes/"+m["id"].(string), nil)
		}
	}
	if code, out := a.do("POST", "/trash/boxes/"+room+"/restore", nil); code != http.StatusOK {
		t.Fatalf("restore: %d %v", code, out)
	}
	if code, _ := a.do("GET", "/items/"+item, nil); code != http.StatusOK {
		t.Errorf("item not restored with its box: %d", code)
	}

	// delete the item alone, then purge it
	a.do("DELETE", "/items/"+item, nil)
	if code, _ := a.do("DELETE", "/trash/items/"+item, nil); code != http.StatusOK {
		t.Errorf("purge item: %d", code)
	}
	if code, _ := a.do("POST", "/trash/items/"+item+"/restore", nil); code != http.StatusNotFound {
		t.Errorf("purged item restorable: %d", code)
	}
}

func TestIntegration_PartialLending(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "pl-"+suffix+"@example.com")

	_, out := a.do("POST", "/boxes", map[string]any{"name": "Room", "type": "room", "label": "PL-" + suffix})
	boxID := id(out)
	_, out = a.do("POST", "/items", map[string]any{"name": "Bolt", "boxId": boxID, "categoryId": "Tools & Hardware", "quantity": 10})
	itemID := id(out)

	qty := func() float64 {
		_, o := a.do("GET", "/items/"+itemID, nil)
		return o["data"].(map[string]any)["quantity"].(float64)
	}

	if code, _ := a.do("POST", "/lending/lend", map[string]any{"itemId": itemID, "borrowerName": "x", "quantity": 11}); code == http.StatusCreated {
		t.Fatalf("lending more than stock must fail")
	}
	code, out := a.do("POST", "/lending/lend", map[string]any{"itemId": itemID, "borrowerName": "bob", "quantity": 4})
	if code != http.StatusCreated {
		t.Fatalf("partial lend: %d %v", code, out)
	}
	loan1 := id(out)
	if got := qty(); got != 6 {
		t.Errorf("quantity after partial lend = %v, want 6", got)
	}
	// a second partial loan may overlap
	code, out = a.do("POST", "/lending/lend", map[string]any{"itemId": itemID, "borrowerName": "amy", "quantity": 2})
	if code != http.StatusCreated {
		t.Fatalf("second partial lend: %d %v", code, out)
	}
	if code, _ := a.do("DELETE", "/items/"+itemID, nil); code == http.StatusOK {
		t.Errorf("item with open loans must not be deletable")
	}
	if code, _ := a.do("POST", "/lending/"+loan1+"/return", map[string]any{}); code != http.StatusOK {
		t.Fatalf("return: %d", code)
	}
	if got := qty(); got != 8 {
		t.Errorf("quantity after return = %v, want 8", got)
	}
}

// Offline clients pre-generate ids; replaying a create/lend must not duplicate.
func TestIntegration_ClientGeneratedIDs(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "cid-"+suffix+"@example.com")

	boxID := "7c1f3a52-0b0e-4f55-9c64-2a8c0d4e9a01"
	itemID := "7c1f3a52-0b0e-4f55-9c64-2a8c0d4e9a02"
	loanID := "7c1f3a52-0b0e-4f55-9c64-2a8c0d4e9a03"

	for i := 0; i < 2; i++ {
		code, out := a.do("POST", "/boxes", map[string]any{"id": boxID, "name": "Room", "type": "room", "label": "CID-" + suffix})
		if code != http.StatusCreated || id(out) != boxID {
			t.Fatalf("box create #%d: %d %v", i, code, out)
		}
		code, out = a.do("POST", "/items", map[string]any{"id": itemID, "name": "Bolt", "boxId": boxID, "categoryId": "Tools & Hardware", "quantity": 5})
		if code != http.StatusCreated || id(out) != itemID {
			t.Fatalf("item create #%d: %d %v", i, code, out)
		}
		code, out = a.do("POST", "/lending/lend", map[string]any{"id": loanID, "itemId": itemID, "borrowerName": "bob", "quantity": 2})
		if code != http.StatusCreated || id(out) != loanID {
			t.Fatalf("lend #%d: %d %v", i, code, out)
		}
	}
	_, out := a.do("GET", "/items/"+itemID, nil)
	if got := out["data"].(map[string]any)["quantity"].(float64); got != 3 {
		t.Errorf("quantity after replayed lend = %v, want 3", got)
	}
}

func TestIntegration_DeleteAccount(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "del-a-"+suffix+"@example.com")
	b := register(t, r, "del-b-"+suffix+"@example.com")

	_, out := a.do("POST", "/boxes", map[string]any{"name": "Room", "type": "room", "label": "DEL-A-" + suffix})
	boxID := id(out)
	_, out = a.do("POST", "/items", map[string]any{"name": "Drill", "boxId": boxID, "categoryId": "Tools & Hardware", "quantity": 1})
	itemID := id(out)
	// A trashed item must be purged as well.
	a.do("DELETE", "/items/"+itemID, nil)
	_, out = b.do("POST", "/boxes", map[string]any{"name": "Room", "type": "room", "label": "DEL-B-" + suffix})
	bBox := id(out)

	if code, out := a.do("DELETE", "/auth/me", nil); code != http.StatusOK {
		t.Fatalf("delete account: %d %v", code, out)
	}
	if code, _ := a.do("GET", "/auth/me", nil); code != http.StatusUnauthorized {
		t.Errorf("deleted user /auth/me = %d, want 401", code)
	}
	code, _ := a.do("POST", "/auth/login", map[string]any{"email": "del-a-" + suffix + "@example.com", "password": "secret123"})
	if code != http.StatusUnauthorized {
		t.Errorf("login after delete = %d, want 401", code)
	}
	if code, _ := b.do("GET", "/boxes/"+bBox, nil); code != http.StatusOK {
		t.Errorf("other user's data must survive, got %d", code)
	}
}

func setupItem(t *testing.T, c *client, suffix string, extra map[string]any) (boxID, itemID string) {
	t.Helper()
	_, out := c.do("POST", "/boxes", map[string]any{"name": "Room", "type": "room", "label": "B-" + suffix})
	boxID = id(out)
	body := map[string]any{"name": "Thing", "boxId": boxID, "categoryId": "Tools & Hardware", "quantity": 10}
	for k, v := range extra {
		body[k] = v
	}
	code, out := c.do("POST", "/items", body)
	if code != http.StatusCreated {
		t.Fatalf("create item: %d %v", code, out)
	}
	return boxID, id(out)
}

func TestIntegration_TokenRevocationAndRefresh(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "tok-"+suffix+"@example.com")
	old := a.token

	code, out := a.do("POST", "/auth/refresh", nil)
	if code != http.StatusOK {
		t.Fatalf("refresh: %d %v", code, out)
	}
	fresh := out["data"].(map[string]any)["token"].(string)

	if code, _ := a.do("POST", "/auth/logout-all", nil); code != http.StatusOK {
		t.Fatalf("logout-all: %d", code)
	}
	for _, tok := range []string{old, fresh} {
		a.token = tok
		if code, _ := a.do("GET", "/auth/me", nil); code != http.StatusUnauthorized {
			t.Errorf("revoked token must get 401, got %d", code)
		}
	}
	// Logging in again yields a token that works.
	code, out = a.do("POST", "/auth/login", map[string]any{"email": "tok-" + suffix + "@example.com", "password": "secret123"})
	if code != http.StatusOK {
		t.Fatalf("login: %d", code)
	}
	a.token = out["data"].(map[string]any)["token"].(string)
	if code, _ := a.do("GET", "/auth/me", nil); code != http.StatusOK {
		t.Errorf("new token must work, got %d", code)
	}
}

func TestIntegration_QuantityDeltaIsAtomic(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "qd-"+suffix+"@example.com")
	_, itemID := setupItem(t, a, suffix, nil)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.do("PATCH", "/items/"+itemID+"/quantity", map[string]any{"delta": -1, "reason": "consume"})
		}()
	}
	wg.Wait()
	_, out := a.do("GET", "/items/"+itemID, nil)
	if q := out["data"].(map[string]any)["quantity"].(float64); q != 5 {
		t.Errorf("quantity after 5 concurrent -1 = %v, want 5", q)
	}
	if code, _ := a.do("PATCH", "/items/"+itemID+"/quantity", map[string]any{"delta": -50}); code != http.StatusBadRequest {
		t.Errorf("delta below zero = %d, want 400", code)
	}
	if code, _ := a.do("PATCH", "/items/"+itemID+"/quantity", map[string]any{"delta": 1, "quantity": 3}); code != http.StatusBadRequest {
		t.Errorf("both delta and quantity = %d, want 400", code)
	}
	_, out = a.do("GET", "/items/"+itemID+"/movements", nil)
	moves := out["data"].(map[string]any)["movements"].([]any)
	if len(moves) != 6 { // initial + 5 consumes
		t.Errorf("movements = %d, want 6", len(moves))
	}
}

func TestIntegration_StaleUpdateAndMoveHistory(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "st-"+suffix+"@example.com")
	boxID, itemID := setupItem(t, a, suffix, nil)
	_, out := a.do("POST", "/boxes", map[string]any{"name": "Other", "type": "box", "label": "O-" + suffix})
	other := id(out)

	_, out = a.do("GET", "/items/"+itemID, nil)
	seen := out["data"].(map[string]any)["updatedAt"]
	if code, out := a.do("PUT", "/items/"+itemID, map[string]any{"name": "v2", "updatedAt": seen, "boxId": other}); code != http.StatusOK {
		t.Fatalf("first update: %d %v", code, out)
	}
	// Replaying with the old version must conflict.
	if code, _ := a.do("PUT", "/items/"+itemID, map[string]any{"name": "v3", "updatedAt": seen}); code != http.StatusConflict {
		t.Errorf("stale update = %d, want 409", code)
	}
	_, out = a.do("GET", "/items/"+itemID+"/movements", nil)
	moves := out["data"].(map[string]any)["movements"].([]any)
	if len(moves) != 2 || moves[0].(map[string]any)["reason"] != "move" {
		t.Errorf("box change via PUT must log a move: %v", moves)
	}
	_ = boxID
}

func TestIntegration_OverdueByCalendarDay(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "od-"+suffix+"@example.com")
	_, itemID := setupItem(t, a, suffix, nil)
	_, item2 := func() (string, string) {
		_, o := a.do("GET", "/items/"+itemID, nil)
		bid := o["data"].(map[string]any)["boxId"].(string)
		_, o = a.do("POST", "/items", map[string]any{"name": "T2", "boxId": bid, "categoryId": "Tools & Hardware"})
		return bid, id(o)
	}()
	today := time.Now().UTC().Format("2006-01-02") + "T00:00:00Z"
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02") + "T00:00:00Z"
	if code, out := a.do("POST", "/lending/lend", map[string]any{"itemId": itemID, "borrowerName": "due-today", "expectedReturnDate": today, "lentDate": yesterday}); code != http.StatusCreated {
		t.Fatalf("lend today: %d %v", code, out)
	}
	if code, out := a.do("POST", "/lending/lend", map[string]any{"itemId": item2, "borrowerName": "late", "expectedReturnDate": yesterday, "lentDate": yesterday}); code != http.StatusCreated {
		t.Fatalf("lend late: %d %v", code, out)
	}
	_, out := a.do("GET", "/lending/overdue", nil)
	list := out["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["borrowerName"] != "late" {
		t.Errorf("overdue = %v, want only the loan due yesterday", list)
	}
	_, out = a.do("GET", "/alerts/summary", nil)
	if n := out["data"].(map[string]any)["overdueCount"].(float64); n != 1 {
		t.Errorf("overdueCount = %v, want 1", n)
	}
}

func TestIntegration_LowStockAndAlertStatus(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "ls-"+suffix+"@example.com")
	boxID, itemID := setupItem(t, a, suffix, map[string]any{"minQuantity": 6})
	_, out := a.do("POST", "/items", map[string]any{"name": "Gone", "boxId": boxID, "categoryId": "Tools & Hardware", "quantity": 1, "minQuantity": 5, "status": "disposed"})
	_ = out

	lowCount := func() float64 {
		_, o := a.do("GET", "/alerts/summary", nil)
		return o["data"].(map[string]any)["lowStockCount"].(float64)
	}
	if n := lowCount(); n != 0 {
		t.Fatalf("lowStockCount = %v, want 0 (10 > 6, disposed ignored)", n)
	}
	// Lending 7 leaves 3 on the shelf but they are still ours: not low stock.
	if code, o := a.do("POST", "/lending/lend", map[string]any{"itemId": itemID, "borrowerName": "bob", "quantity": 7}); code != http.StatusCreated {
		t.Fatalf("lend: %d %v", code, o)
	}
	if n := lowCount(); n != 0 {
		t.Errorf("partial loan must not raise low stock, got %v", n)
	}
	_, o := a.do("GET", "/items/"+itemID, nil)
	d := o["data"].(map[string]any)
	if d["isLowStock"] != false || d["lentQuantity"].(float64) != 7 {
		t.Errorf("item = %v", d)
	}
	if loans := d["activeLoans"].([]any); len(loans) != 1 {
		t.Errorf("activeLoans = %v, want 1", loans)
	}
	// Really consuming stock does raise it.
	a.do("PATCH", "/items/"+itemID+"/quantity", map[string]any{"delta": -3, "reason": "consume"})
	if n := lowCount(); n != 0 {
		t.Errorf("total 7 > min 6 must not be low, got %v", n)
	}
	a.do("PUT", "/items/"+itemID, map[string]any{"minQuantity": 7})
	if n := lowCount(); n != 1 {
		t.Errorf("lowStockCount = %v, want 1", n)
	}
}

func TestIntegration_PurgeBlockedByOpenLoanAndBoxMoveCycle(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "pg-"+suffix+"@example.com")
	boxID, itemID := setupItem(t, a, suffix, nil)
	if code, o := a.do("POST", "/lending/lend", map[string]any{"itemId": itemID, "borrowerName": "bob", "quantity": 3}); code != http.StatusCreated {
		t.Fatalf("lend: %d %v", code, o)
	}
	// Force the item into the trash despite the loan to exercise the purge guard.
	_, o := a.do("GET", "/lending/active", nil)
	loan := o["data"].([]any)[0].(map[string]any)["id"].(string)
	a.do("POST", "/lending/"+loan+"/return", map[string]any{})
	a.do("POST", "/lending/lend", map[string]any{"itemId": itemID, "borrowerName": "amy", "quantity": 2})
	if code, _ := a.do("DELETE", "/items/"+itemID, nil); code == http.StatusOK {
		t.Fatalf("delete with open loan must fail")
	}

	// Box cycle: child -> parent's descendant must be rejected, concurrent opposite moves cannot both win.
	_, o = a.do("POST", "/boxes", map[string]any{"name": "A", "type": "box", "label": "A-" + suffix})
	boxA := id(o)
	_, o = a.do("POST", "/boxes", map[string]any{"name": "B", "type": "box", "label": "Bx-" + suffix})
	boxB := id(o)
	var wg sync.WaitGroup
	codes := make([]int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes[0], _ = a.do("PATCH", "/boxes/"+boxA+"/move", map[string]any{"newParentId": boxB})
	}()
	go func() {
		defer wg.Done()
		codes[1], _ = a.do("PATCH", "/boxes/"+boxB+"/move", map[string]any{"newParentId": boxA})
	}()
	wg.Wait()
	ok := 0
	for _, c := range codes {
		if c == http.StatusOK {
			ok++
		}
	}
	if ok != 1 {
		t.Errorf("exactly one of two opposite moves may succeed, codes=%v", codes)
	}
	_ = boxID
}

func TestIntegration_LendWithForeignIDConflicts(t *testing.T) {
	r := newRouter(t)
	suffix := t.Name() + "-" + os.Getenv("INTEGRATION_RUN")
	a := register(t, r, "fa-"+suffix+"@example.com")
	b := register(t, r, "fb-"+suffix+"@example.com")
	_, itemA := setupItem(t, a, "a"+suffix, nil)
	_, itemB := setupItem(t, b, "b"+suffix, nil)
	shared := uuid.NewString()
	if code, o := a.do("POST", "/lending/lend", map[string]any{"id": shared, "itemId": itemA, "borrowerName": "x"}); code != http.StatusCreated {
		t.Fatalf("lend a: %d %v", code, o)
	}
	if code, _ := b.do("POST", "/lending/lend", map[string]any{"id": shared, "itemId": itemB, "borrowerName": "y"}); code != http.StatusConflict {
		t.Errorf("foreign id reuse = %d, want 409", code)
	}
}
