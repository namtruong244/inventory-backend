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

		// Ensure nullable password_hash and columns for social login
		_ = db.Exec("ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;").Error
		_ = db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS provider VARCHAR(50) DEFAULT 'local';").Error
		_ = db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS provider_id VARCHAR(255) NULL;").Error

		// Create performance and GIN indexes
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_boxes_user_parent ON boxes(user_id, parent_id);").Error
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_boxes_label ON boxes(label);").Error
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_items_user_box ON items(user_id, box_id);").Error
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_items_barcode ON items(barcode);").Error
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_items_serial ON items(serial_number);").Error
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_items_status ON items(status);").Error
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_items_custom_attributes ON items USING gin (custom_attributes);").Error
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_lending_active ON lending_records(user_id, actual_return_date);").Error

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
