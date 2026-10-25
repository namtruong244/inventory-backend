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

			// Box labels are unique per user (scan lookup resolves labels within one user)
			"CREATE UNIQUE INDEX IF NOT EXISTS uq_boxes_user_label ON boxes(user_id, label);",
			// An item can have at most one open loan (guards against concurrent lend requests)
			"CREATE UNIQUE INDEX IF NOT EXISTS uq_lending_open_item ON lending_records(item_id) WHERE actual_return_date IS NULL;",

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

// addForeignKey returns an idempotent statement adding a foreign key.
func addForeignKey(name, table, column, refTable, onDelete string) string {
	return fmt.Sprintf(`DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '%s') THEN
			ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(id) ON DELETE %s;
		END IF;
	END $$;`, name, table, name, column, refTable, onDelete)
}
