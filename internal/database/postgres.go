package database

import (
	"fmt"
	"log"
	"time"

	"inventory_backend/internal/config"
	"inventory_backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort, cfg.DBSSLMode,
	)

	gormLogger := logger.Default.LogMode(logger.Warn)
	if cfg.AppEnv == "development" {
		gormLogger = logger.Default.LogMode(logger.Info)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB handle: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if cfg.RunMigrations {
		// Auto-migrate tables
		log.Println("Running database migrations...")
		err = db.AutoMigrate(
			&models.User{},
			&models.UserSettings{},
			&models.Category{},
			&models.Box{},
			&models.Item{},
			&models.LendingRecord{},
			&models.StockMovement{},
		)
		if err != nil {
			return nil, fmt.Errorf("database migration failed: %w", err)
		}

		// Every statement is idempotent. A failure aborts startup: running with a
		// half-applied schema (e.g. a missing unique index) silently breaks invariants.
		statements := []string{
			// Nullable password_hash and columns for social login
			"ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS provider VARCHAR(50) DEFAULT 'local';",
			"ALTER TABLE users ADD COLUMN IF NOT EXISTS provider_id VARCHAR(255) NULL;",

			// Performance and GIN indexes
			"CREATE INDEX IF NOT EXISTS idx_boxes_user_parent ON boxes(user_id, parent_id);",
			"CREATE INDEX IF NOT EXISTS idx_boxes_label ON boxes(label);",
			"CREATE INDEX IF NOT EXISTS idx_items_user_box ON items(user_id, box_id);",
			"CREATE INDEX IF NOT EXISTS idx_items_barcode ON items(barcode);",
			"CREATE INDEX IF NOT EXISTS idx_items_serial ON items(serial_number);",
			"CREATE INDEX IF NOT EXISTS idx_items_status ON items(status);",
			"CREATE INDEX IF NOT EXISTS idx_items_custom_attributes ON items USING gin (custom_attributes);",
			"CREATE INDEX IF NOT EXISTS idx_lending_active ON lending_records(user_id, actual_return_date);",

			"CREATE INDEX IF NOT EXISTS idx_items_user_status ON items(user_id, status);",
			"CREATE INDEX IF NOT EXISTS idx_items_user_expiry ON items(user_id, expiry_date);",
			"CREATE INDEX IF NOT EXISTS idx_items_user_warranty ON items(user_id, warranty_expiry_date);",
			"CREATE INDEX IF NOT EXISTS idx_items_user_created ON items(user_id, created_at DESC);",

			// Box labels are unique per user (scan lookup resolves labels within one user)
			// Soft-deleted boxes keep their label, so uniqueness only covers live rows.
			"DROP INDEX IF EXISTS uq_boxes_user_label;",
			"CREATE UNIQUE INDEX IF NOT EXISTS uq_boxes_user_label_live ON boxes(user_id, label) WHERE deleted_at IS NULL;",
			// An item can have at most one open loan (guards against concurrent lend requests)
			"DROP INDEX IF EXISTS uq_lending_open_item;",
			// Only whole-item loans are exclusive; partial loans may overlap.
			"CREATE UNIQUE INDEX IF NOT EXISTS uq_lending_open_whole ON lending_records(item_id) WHERE actual_return_date IS NULL AND is_partial = false;",

			// Referential integrity: no orphaned items, sub-boxes or loans
			addForeignKey("fk_items_box", "items", "box_id", "boxes", "NO ACTION"),
			addForeignKey("fk_boxes_parent", "boxes", "parent_id", "boxes", "NO ACTION"),
			addForeignKey("fk_lending_item", "lending_records", "item_id", "items", "CASCADE"),
		}
		for _, stmt := range statements {
			if err := db.Exec(stmt).Error; err != nil {
				return nil, fmt.Errorf("database migration statement failed (%.60s...): %w", stmt, err)
			}
		}

		// Trigram indexes make ILIKE '%q%' search index-assisted. pg_trgm needs a
		// privileged role on some hosts, so this is best-effort: search still works without it.
		for _, stmt := range []string{
			"CREATE EXTENSION IF NOT EXISTS pg_trgm;",
			"CREATE INDEX IF NOT EXISTS idx_items_name_trgm ON items USING gin (name gin_trgm_ops);",
			"CREATE INDEX IF NOT EXISTS idx_items_description_trgm ON items USING gin (description gin_trgm_ops);",
		} {
			if err := db.Exec(stmt).Error; err != nil {
				log.Printf("Warning: optional search index skipped (%.50s...): %v\n", stmt, err)
				break
			}
		}

		normalizeUserEmails(db)

		// Seed system defaults
		if err := SeedDefaultCategories(db); err != nil {
			log.Printf("Warning: Seeding default categories failed: %v\n", err)
		}

		log.Println("Database migration & seed completed successfully.")
	} else {
		log.Println("Skipping database migrations (RUN_MIGRATIONS=false)")
	}
	return db, nil
}

// normalizeUserEmails lower-cases stored emails and makes them unique
// case-insensitively. Accounts that only differ by case cannot be merged
// automatically (they own separate data), so when such duplicates exist this
// only logs them for manual cleanup and retries on the next start.
func normalizeUserEmails(db *gorm.DB) {
	var dups []string
	if err := db.Raw(`SELECT LOWER(TRIM(email)) FROM users GROUP BY LOWER(TRIM(email)) HAVING COUNT(*) > 1`).Scan(&dups).Error; err != nil {
		log.Printf("Warning: email duplicate check skipped: %v\n", err)
		return
	}
	if len(dups) > 0 {
		log.Printf("Warning: users with emails differing only by case/whitespace, merge them then restart: %v\n", dups)
		return
	}
	for _, stmt := range []string{
		"UPDATE users SET email = LOWER(TRIM(email)) WHERE email <> LOWER(TRIM(email));",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_users_email_lower ON users (LOWER(email));",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			log.Printf("Warning: email normalization skipped (%.50s...): %v\n", stmt, err)
			return
		}
	}
}

// addForeignKey returns an idempotent statement adding a foreign key.
func addForeignKey(name, table, column, refTable, onDelete string) string {
	return fmt.Sprintf(`DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '%s') THEN
			ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(id) ON DELETE %s;
		END IF;
	END $$;`, name, table, name, column, refTable, onDelete)
}
