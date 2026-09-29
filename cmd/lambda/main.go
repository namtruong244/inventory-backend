package main

import (
	"context"
	"encoding/json"
	"log"

	"inventory_backend/internal/app"
	"inventory_backend/internal/config"
	"inventory_backend/internal/database"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
)

var (
	adapterV1 *ginadapter.GinLambda
	adapterV2 *ginadapter.GinLambdaV2
)

func init() {
	log.Println("Initializing Inventory Backend on AWS Lambda...")

	// 1. Load Configuration
	cfg := config.LoadConfig()

	// 2. Initialize Database
	db, err := database.InitDB(cfg)
	if err != nil {
		log.Fatalf("Fatal: Database initialization failed on Lambda cold start: %v", err)
	}

	// 3. Setup Gin Router
	router, err := app.SetupRouter(cfg, db)
	if err != nil {
		log.Fatalf("Fatal: Router initialization failed on Lambda cold start: %v", err)
	}

	// 4. Create adapters for both API Gateway HTTP API (v2) and REST API (v1)
	adapterV1 = ginadapter.New(router)
	adapterV2 = ginadapter.NewV2(router)

	log.Printf("Lambda initialized successfully (env: %s, storage: %s)", cfg.AppEnv, cfg.StorageDriver)
}

type eventVersion struct {
	Version string `json:"version"`
}

// UniversalHandler automatically handles API Gateway HTTP API (v2), Function URLs, and REST API (v1).
func UniversalHandler(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	var detector eventVersion
	if err := json.Unmarshal(raw, &detector); err == nil && detector.Version == "2.0" {
		var req events.APIGatewayV2HTTPRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return adapterV2.ProxyWithContext(ctx, req)
	}

	var req events.APIGatewayProxyRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	return adapterV1.ProxyWithContext(ctx, req)
}

func main() {
	lambda.Start(UniversalHandler)
}
