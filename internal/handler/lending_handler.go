package handler

import (
	"net/http"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type LendingHandler struct {
	lendingService service.LendingService
}

func NewLendingHandler(lendingService service.LendingService) *LendingHandler {
	return &LendingHandler{lendingService: lendingService}
}

func (h *LendingHandler) Lend(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	var req service.LendItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	record, err := h.lendingService.LendItem(userID, req)
	if err != nil {
		SendError(c, http.StatusBadRequest, "LEND_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusCreated, record, "Item lent successfully")
}

func (h *LendingHandler) Return(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid record ID format", nil)
		return
	}

	err = h.lendingService.ReturnItem(userID, id)
	if err != nil {
		SendError(c, http.StatusBadRequest, "RETURN_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, gin.H{"id": id}, "Item returned successfully")
}

func (h *LendingHandler) Active(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	records, err := h.lendingService.GetActiveLoans(userID)
	if err != nil {
		SendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, records, "")
}

func (h *LendingHandler) Overdue(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	records, err := h.lendingService.GetOverdueLoans(userID)
	if err != nil {
		SendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, records, "")
}

func (h *LendingHandler) History(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	itemIDStr := c.Query("itemId")
	if itemIDStr == "" {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", "itemId query parameter is required", nil)
		return
	}

	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid itemId format", nil)
		return
	}

	records, err := h.lendingService.GetHistoryByItemID(userID, itemID)
	if err != nil {
		SendError(c, http.StatusBadRequest, "HISTORY_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, records, "")
}
