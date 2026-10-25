package handler

import (
	"net/http"
	"time"

	"inventory_backend/internal/apperr"
	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService service.AuthService
	// loginByEmail throttles guessing against a single account regardless of IP.
	loginByEmail *middleware.RateLimiter
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService:  authService,
		loginByEmail: middleware.NewRateLimiter(10, 15*time.Minute),
	}
}

type RegisterRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type GoogleLoginRequest struct {
	Email            string  `json:"email" binding:"required,email"`
	Name             string  `json:"name" binding:"required"`
	AvatarURL        *string `json:"avatar_url"`
	AvatarUrl        *string `json:"avatarUrl"`
	IDToken          *string `json:"id_token"`
	IdToken          *string `json:"idToken"`
	AccessToken      *string `json:"access_token"`
	AccessTokenCamel *string `json:"accessToken"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", "Invalid request body", nil)
		return
	}

	result, err := h.authService.Register(req.Name, req.Email, req.Password)
	if err != nil {
		fail(c, "REGISTRATION_FAILED", err)
		return
	}

	SendSuccess(c, http.StatusCreated, result, "Account registered successfully")
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", "Invalid request body", nil)
		return
	}

	// Only failed attempts count, and per (email, IP): a stranger cannot lock
	// the owner out, and a successful login clears the counter.
	limitKey := middleware.EmailIPKey(req.Email, c.ClientIP())
	if h.loginByEmail.Blocked(limitKey) {
		middleware.Reject(c)
		return
	}

	result, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		h.loginByEmail.Allow(limitKey)
		if e, ok := apperr.As(err); ok {
			SendError(c, http.StatusUnauthorized, "AUTH_FAILED", e.Msg, nil)
			return
		}
		fail(c, "AUTH_FAILED", err)
		return
	}

	h.loginByEmail.Reset(limitKey)
	SendSuccess(c, http.StatusOK, result, "Login successful")
}

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	var req GoogleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", "Invalid request body", nil)
		return
	}

	avatarURL := req.AvatarURL
	if avatarURL == nil && req.AvatarUrl != nil {
		avatarURL = req.AvatarUrl
	}

	idToken := req.IDToken
	if idToken == nil && req.IdToken != nil {
		idToken = req.IdToken
	}

	result, err := h.authService.LoginWithGoogle(req.Email, req.Name, avatarURL, idToken)
	if err != nil {
		fail(c, "AUTH_FAILED", err)
		return
	}

	SendSuccess(c, http.StatusOK, result, "Google authentication successful")
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	user, err := h.authService.GetProfile(userID)
	if err != nil {
		SendError(c, http.StatusNotFound, "USER_NOT_FOUND", "User profile not found", nil)
		return
	}

	SendSuccess(c, http.StatusOK, service.UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		AvatarURL: user.AvatarURL,
		Provider:  user.Provider,
	}, "")
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}
	result, err := h.authService.Refresh(userID)
	if err != nil {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not found", nil)
		return
	}
	SendSuccess(c, http.StatusOK, result, "Token refreshed")
}

// LogoutAll revokes every token of the caller (this device included).
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}
	if err := h.authService.RevokeAllTokens(userID); err != nil {
		fail(c, "LOGOUT_FAILED", err)
		return
	}
	SendSuccess(c, http.StatusOK, nil, "All sessions signed out")
}

func (h *AuthHandler) DeleteMe(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	if err := h.authService.DeleteAccount(userID); err != nil {
		fail(c, "DELETE_FAILED", err)
		return
	}

	SendSuccess(c, http.StatusOK, nil, "Account deleted")
}
