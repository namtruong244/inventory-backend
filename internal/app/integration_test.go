package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"inventory_backend/internal/app"
	"inventory_backend/internal/config"
	"inventory_backend/internal/database"

	"github.com/gin-gonic/gin"
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
