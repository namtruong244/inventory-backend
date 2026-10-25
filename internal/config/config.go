package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	AppEnv             string
	BaseURL            string
	DBHost             string
	DBPort             string
	DBUser             string
	DBPassword         string
	DBName             string
	DBSSLMode          string
	DBMaxOpenConns     int
	DBMaxIdleConns     int
	RunMigrations      bool
	JWTSecret          string
	JWTExpirationHours int
	UploadDir          string
	CORSAllowedOrigins []string // empty: allow all in non-production, none in production

	// Storage configuration
	StorageDriver     string // "local" or "r2"
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketName      string
	R2PublicURL       string

	// OAuth & Social Auth
	GoogleClientID string
}

func LoadConfig() *Config {
	// Attempt to load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("Note: .env file not found or failed to load, reading from environment variables")
	}

	jwtExpHours, err := strconv.Atoi(getEnv("JWT_EXPIRATION_HOURS", "720"))
	if err != nil {
		jwtExpHours = 720
	}

	dbMaxOpenConns, err := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "25"))
	if err != nil {
		dbMaxOpenConns = 25
	}

	dbMaxIdleConns, err := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "5"))
	if err != nil {
		dbMaxIdleConns = 5
	}

	runMigrations := getEnvAsBool("RUN_MIGRATIONS", true)

	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		AppEnv:             getEnv("APP_ENV", "development"),
		BaseURL:            getEnv("BASE_URL", "http://localhost:8080"),
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnv("DB_PORT", "5432"),
		DBUser:             getEnv("DB_USER", "postgres"),
		DBPassword:         getEnv("DB_PASSWORD", "postgres"),
		DBName:             getEnv("DB_NAME", "inventory_db"),
		DBSSLMode:          getEnv("DB_SSLMODE", "disable"),
		DBMaxOpenConns:     dbMaxOpenConns,
		DBMaxIdleConns:     dbMaxIdleConns,
		RunMigrations:      runMigrations,
		JWTSecret:          getEnv("JWT_SECRET", insecureDefaultJWTSecret),
		JWTExpirationHours: jwtExpHours,
		UploadDir:          getEnv("UPLOAD_DIR", "./uploads"),
		CORSAllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "")),
		StorageDriver:      getEnv("STORAGE_DRIVER", "local"),
		R2AccountID:        getEnv("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:      getEnv("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:  getEnv("R2_SECRET_ACCESS_KEY", ""),
		R2BucketName:       getEnv("R2_BUCKET_NAME", ""),
		R2PublicURL:        getEnv("R2_PUBLIC_URL", ""),
		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", "1059613381730-sbh1t9581ud40drbiu8ssh7fj1e3ahev.apps.googleusercontent.com"),
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Fatal: invalid configuration: %v", err)
	}
	return cfg
}

const (
	insecureDefaultJWTSecret = "default_secret_inventory_system_key"
	minProductionJWTSecret   = 32
)

// Validate rejects configurations that are unsafe to run in production.
func (c *Config) Validate() error {
	if c.AppEnv != "production" {
		return nil
	}
	if c.JWTSecret == insecureDefaultJWTSecret || strings.Contains(c.JWTSecret, "change_in_production") {
		return errors.New("JWT_SECRET must be set to a unique secret in production")
	}
	if len(c.JWTSecret) < minProductionJWTSecret {
		return fmt.Errorf("JWT_SECRET must be at least %d characters in production", minProductionJWTSecret)
	}
	return nil
}

func splitCSV(val string) []string {
	var out []string
	for _, part := range strings.Split(val, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func getEnv(key, defaultVal string) string {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		boolVal, err := strconv.ParseBool(val)
		if err == nil {
			return boolVal
		}
	}
	return defaultVal
}
