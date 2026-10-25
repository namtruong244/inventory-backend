package middleware

import (
	"net/http"
	"strings"

	"inventory_backend/internal/response"
	"inventory_backend/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	ContextUserIDKey = "userID"
	ContextEmailKey  = "userEmail"
	ContextRoleKey   = "userRole"
)

// TokenVerifier reports whether a validly signed token is still accepted, i.e.
// the user exists and the token carries their current token version. A non-nil
// error means the check itself failed (e.g. database outage), not that the
// token is revoked.
type TokenVerifier func(userID uuid.UUID, tokenVersion int) (bool, error)

func AuthMiddleware(jwtSecret string, verify ...TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization header is required", nil)
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			response.SendError(c, http.StatusUnauthorized, "INVALID_TOKEN_FORMAT", "Authorization header must be Bearer <token>", nil)
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims, err := utils.ValidateToken(tokenString, jwtSecret)
		if err != nil {
			response.SendError(c, http.StatusUnauthorized, "TOKEN_EXPIRED_OR_INVALID", "Token is invalid or has expired", nil)
			c.Abort()
			return
		}

		for _, v := range verify {
			ok, verr := v(claims.UserID, claims.TokenVersion)
			if verr != nil {
				response.SendError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Could not verify the session, please try again", nil)
				c.Abort()
				return
			}
			if !ok {
				response.SendError(c, http.StatusUnauthorized, "TOKEN_REVOKED", "Token has been revoked, please sign in again", nil)
				c.Abort()
				return
			}
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Set(ContextEmailKey, claims.Email)
		c.Set(ContextRoleKey, claims.Role)
		c.Next()
	}
}

func GetCurrentUserID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get(ContextUserIDKey)
	if !exists {
		return uuid.Nil, false
	}
	userID, ok := val.(uuid.UUID)
	return userID, ok
}
