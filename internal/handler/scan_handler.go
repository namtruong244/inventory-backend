package handler

import (
	"net/http"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
)

type ScanHandler struct {
	scanService service.ScanService
}

func NewScanHandler(scanService service.ScanService) *ScanHandler {
	return &ScanHandler{scanService: scanService}
}

func (h *ScanHandler) Lookup(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	code := c.Query("code")
	if code == "" {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", "code query parameter is required", nil)
		return
	}

	result, err := h.scanService.Lookup(userID, code)
	if err != nil {
		SendError(c, http.StatusNotFound, "NOT_FOUND", err.Error(), []ErrorDetail{
			{Field: "code", Message: "No box or item found matching code " + code},
		})
		return
	}

	SendSuccess(c, http.StatusOK, result, "")
}
