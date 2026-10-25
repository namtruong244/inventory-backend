package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORSMiddleware allows the given origins. Auth uses a Bearer header, so credentials
// (cookies) are never allowed. With no origins configured, every origin is allowed
// unless production is true, in which case cross-origin browser access is disabled
// (the mobile app does not need CORS).
func CORSMiddleware(allowedOrigins []string, production bool) gin.HandlerFunc {
	cfg := cors.Config{
		AllowMethods:  []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders: []string{"Content-Length", "Content-Disposition"},
		MaxAge:        12 * time.Hour,
	}
	switch {
	case len(allowedOrigins) > 0:
		cfg.AllowOrigins = allowedOrigins
	case !production:
		cfg.AllowAllOrigins = true
	default:
		return func(c *gin.Context) { c.Next() }
	}
	return cors.New(cfg)
}
