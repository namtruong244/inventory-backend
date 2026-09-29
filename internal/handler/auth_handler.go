package handler

import (
	"net/http"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService service.AuthService
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type RegisterRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	result, err := h.authService.Register(req.Name, req.Email, req.Password)
	if err != nil {
		SendError(c, http.StatusBadRequest, "REGISTRATION_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusCreated, result, "Account registered successfully")
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	result, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		SendError(c, http.StatusUnauthorized, "AUTH_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, result, "Login successful")
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
	}, "")
}
