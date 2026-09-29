package handler

import (
	"net/http"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
)

type SettingsHandler struct {
	settingsService service.SettingsService
}

func NewSettingsHandler(settingsService service.SettingsService) *SettingsHandler {
	return &SettingsHandler{settingsService: settingsService}
}

func (h *SettingsHandler) GetSettings(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	settings, err := h.settingsService.GetSettings(userID)
	if err != nil {
		SendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve user settings", nil)
		return
	}

	SendSuccess(c, http.StatusOK, settings, "")
}

func (h *SettingsHandler) UpdateSettings(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	var req service.UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	settings, err := h.settingsService.UpdateSettings(userID, req)
	if err != nil {
		SendError(c, http.StatusBadRequest, "UPDATE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, settings, "Settings updated successfully")
}

func (h *SettingsHandler) UpdateProfile(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	var req service.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	user, err := h.settingsService.UpdateProfile(userID, req)
	if err != nil {
		SendError(c, http.StatusBadRequest, "UPDATE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, service.UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		AvatarURL: user.AvatarURL,
	}, "Profile updated successfully")
}
