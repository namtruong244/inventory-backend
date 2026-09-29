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
