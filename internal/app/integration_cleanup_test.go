package app_test

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"inventory_backend/internal/app"
	"inventory_backend/internal/config"
	"inventory_backend/internal/database"

	"github.com/gin-gonic/gin"
)

func TestIntegration_EmailCaseInsensitiveAndTrashPurge(t *testing.T) {
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
		t.Fatal(err)
	}
	r, err := app.SetupRouter(cfg, db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := os.Getenv("INTEGRATION_RUN")
	email := "Case-" + suffix + "@Example.com"
	a := register(t, r, email)
	c := &client{t: t, router: r}
	code, out := c.do("POST", "/auth/register", map[string]any{"name": "u", "email": strings.ToLower(email), "password": "secret123"})
	if code != http.StatusBadRequest {
		t.Fatalf("duplicate in other case: %d %v", code, out)
	}
	if code, _ := c.do("POST", "/auth/login", map[string]any{"email": strings.ToUpper(email), "password": "secret123"}); code != http.StatusOK {
		t.Fatalf("login in other case: %d", code)
	}
	if code, out := c.do("GET", "/boxes", nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d %v", code, out)
	}

	_, bo := a.do("POST", "/boxes", map[string]any{"name": "Old", "type": "box", "label": "OLD-" + suffix})
	box := id(bo)
	_, io := a.do("POST", "/items", map[string]any{"name": "Gone", "boxId": box, "categoryId": "Tools & Hardware"})
	item := id(io)
	if code, _ := a.do("DELETE", "/boxes/"+box+"?cascade=true", nil); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	purge, err := app.NewTrashPurger(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	// Fresh trash is kept.
	if _, err := purge(); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Raw("SELECT COUNT(*) FROM items WHERE id = ?", item).Scan(&n)
	if n != 1 {
		t.Fatalf("recent trash was purged")
	}
	db.Exec("UPDATE items SET deleted_at = NOW() - INTERVAL '31 days' WHERE id = ?", item)
	db.Exec("UPDATE boxes SET deleted_at = NOW() - INTERVAL '31 days' WHERE id = ?", box)
	if _, err := purge(); err != nil {
		t.Fatal(err)
	}
	db.Raw("SELECT COUNT(*) FROM items WHERE id = ?", item).Scan(&n)
	if n != 0 {
		t.Fatalf("expired item not purged")
	}
	db.Raw("SELECT COUNT(*) FROM boxes WHERE id = ?", box).Scan(&n)
	if n != 0 {
		t.Fatalf("expired box not purged")
	}
}
