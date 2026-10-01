package app

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"inventory_backend/internal/config"
	"inventory_backend/internal/handler"
	"inventory_backend/internal/middleware"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/service"
	"inventory_backend/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupRouter initializes repositories, services, handlers, and the Gin engine.
func SetupRouter(cfg *config.Config, db *gorm.DB) (*gin.Engine, error) {
	// Set Gin mode
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 1. Storage Provider
	storageProvider, err := storage.NewStorageProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage provider: %w", err)
	}

	// 2. Initialize Repositories
	userRepo := repository.NewUserRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	boxRepo := repository.NewBoxRepository(db)
	itemRepo := repository.NewItemRepository(db)
	lendingRepo := repository.NewLendingRepository(db)

	// 3. Initialize Services
	authService := service.NewAuthService(userRepo, settingsRepo, cfg)
	settingsService := service.NewSettingsService(settingsRepo, userRepo)
	categoryService := service.NewCategoryService(categoryRepo)
	boxService := service.NewBoxService(boxRepo, itemRepo)
	itemService := service.NewItemService(itemRepo, boxRepo, lendingRepo, settingsRepo)
	lendingService := service.NewLendingService(lendingRepo, itemRepo)
	scanService := service.NewScanService(boxRepo, itemRepo)
	alertService := service.NewAlertService(db, settingsRepo, lendingRepo)
	reportService := service.NewReportService(db, itemRepo, boxRepo, settingsRepo)
	mediaService := service.NewMediaService(storageProvider)

	// 4. Initialize Handlers
	authHandler := handler.NewAuthHandler(authService)
	settingsHandler := handler.NewSettingsHandler(settingsService)
	categoryHandler := handler.NewCategoryHandler(categoryService)
	boxHandler := handler.NewBoxHandler(boxService)
	itemHandler := handler.NewItemHandler(itemService)
	lendingHandler := handler.NewLendingHandler(lendingService)
	scanHandler := handler.NewScanHandler(scanService)
	alertHandler := handler.NewAlertHandler(alertService)
	reportHandler := handler.NewReportHandler(reportService)
	mediaHandler := handler.NewMediaHandler(mediaService)

	// 5. Setup Gin Router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.Use(middleware.CORSMiddleware())

	// Static file serving only if using local storage
	if strings.ToLower(cfg.StorageDriver) == "local" || cfg.StorageDriver == "" {
		router.Static("/uploads", cfg.UploadDir)
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		storageType := cfg.StorageDriver
		if storageType == "" {
			storageType = "local"
		}
		handler.SendSuccess(c, http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"version":   "1.0.0",
			"storage":   storageType,
			"env":       cfg.AppEnv,
		}, "Inventory Backend API is running")
	})

	// API v1 Group
	v1 := router.Group("/api/v1")
	{
		// Public Authentication Routes
		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/google", authHandler.GoogleLogin)
			authGroup.POST("/apple", authHandler.AppleLogin)
		}

		// Protected Routes
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware(cfg.JWTSecret))
		{
			// Auth Profile
			protected.GET("/auth/me", authHandler.Me)

			// Settings & Profile
			protected.GET("/settings", settingsHandler.GetSettings)
			protected.PATCH("/settings", settingsHandler.UpdateSettings)
			protected.PATCH("/user/profile", settingsHandler.UpdateProfile)

			// Categories
			categoriesGroup := protected.Group("/categories")
			{
				categoriesGroup.GET("", categoryHandler.List)
				categoriesGroup.POST("", categoryHandler.Create)
				categoriesGroup.DELETE("/:name", categoryHandler.Delete)
			}

			// Boxes / Storage Spaces
			boxesGroup := protected.Group("/boxes")
			{
				boxesGroup.GET("", boxHandler.List)
				boxesGroup.POST("", boxHandler.Create)
				boxesGroup.GET("/:id", boxHandler.GetByID)
				boxesGroup.PUT("/:id", boxHandler.Update)
				boxesGroup.PATCH("/:id/move", boxHandler.Move)
				boxesGroup.DELETE("/:id", boxHandler.Delete)
			}

			// Items / Asset Catalog
			itemsGroup := protected.Group("/items")
			{
				itemsGroup.GET("", itemHandler.List)
				itemsGroup.POST("", itemHandler.Create)
				itemsGroup.GET("/:id", itemHandler.GetByID)
				itemsGroup.PUT("/:id", itemHandler.Update)
				itemsGroup.PATCH("/:id/move", itemHandler.Move)
				itemsGroup.PATCH("/:id/quantity", itemHandler.UpdateQuantity)
				itemsGroup.DELETE("/:id", itemHandler.Delete)
			}

			// Lending & Borrowing Tracker
			lendingGroup := protected.Group("/lending")
			{
				lendingGroup.POST("/lend", lendingHandler.Lend)
				lendingGroup.POST("/:id/return", lendingHandler.Return)
				lendingGroup.GET("/active", lendingHandler.Active)
				lendingGroup.GET("/overdue", lendingHandler.Overdue)
				lendingGroup.GET("/history", lendingHandler.History)
			}

			// Barcode & QR Code Scanning
			scanGroup := protected.Group("/scan")
			{
				scanGroup.GET("/lookup", scanHandler.Lookup)
			}

			// Alerts & Dashboard
			alertsGroup := protected.Group("/alerts")
			{
				alertsGroup.GET("/summary", alertHandler.Summary)
				alertsGroup.GET("/all", alertHandler.All)
			}

			// Reports & Inventory Audit
			reportsGroup := protected.Group("/reports")
			{
				reportsGroup.GET("/inventory-health", reportHandler.InventoryHealth)
				reportsGroup.GET("/export", reportHandler.Export)
			}

			// Media Upload
			mediaGroup := protected.Group("/media")
			{
				mediaGroup.POST("/upload", mediaHandler.Upload)
			}
		}
	}

	return router, nil
}
