package handler

import (
	"net/http"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
)

type AlertHandler struct {
	alertService service.AlertService
}

func NewAlertHandler(alertService service.AlertService) *AlertHandler {
	return &AlertHandler{alertService: alertService}
}

func (h *AlertHandler) Summary(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	summary, err := h.alertService.GetSummary(userID)
	if err != nil {
		fail(c, "INTERNAL_ERROR", err)
		return
	}

	SendSuccess(c, http.StatusOK, summary, "")
}

func (h *AlertHandler) All(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	alerts, err := h.alertService.GetAll(userID)
	if err != nil {
		fail(c, "INTERNAL_ERROR", err)
		return
	}

	SendSuccess(c, http.StatusOK, alerts, "")
}
