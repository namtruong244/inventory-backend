package handler

import (
	"net/http"
	"strconv"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/models"
	"inventory_backend/internal/repository"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ItemHandler struct {
	itemService service.ItemService
}

func NewItemHandler(itemService service.ItemService) *ItemHandler {
	return &ItemHandler{itemService: itemService}
}

type MoveItemRequest struct {
	TargetBoxID uuid.UUID `json:"targetBoxId" binding:"required"`
}

type UpdateQuantityRequest struct {
	Quantity float64 `json:"quantity" binding:"required"`
}

func (h *ItemHandler) List(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	filter := repository.ItemFilter{
		Q:        c.Query("q"),
		Category: c.Query("category"),
		Status:   c.Query("status"),
	}

	if boxIDStr := c.Query("boxId"); boxIDStr != "" {
		if parsed, err := uuid.Parse(boxIDStr); err == nil {
			filter.BoxID = &parsed
		}
	}

	if lowStockStr := c.Query("isLowStock"); lowStockStr != "" {
		val := lowStockStr == "true"
		filter.IsLowStock = &val
	}

	if expiredStr := c.Query("isExpired"); expiredStr != "" {
		val := expiredStr == "true"
		filter.IsExpired = &val
	}

	if expiringStr := c.Query("isExpiring"); expiringStr != "" {
		val := expiringStr == "true"
		filter.IsExpiring = &val
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 {
		limit = 20
	}
	filter.Page = page
	filter.Limit = limit

	items, total, err := h.itemService.List(userID, filter)
	if err != nil {
		SendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	if items == nil {
		items = []models.Item{} // ensure non-nil in json
	}

	meta := &Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	SendSuccessWithMeta(c, http.StatusOK, items, meta, "")
}

func (h *ItemHandler) Create(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	var req service.CreateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	item, err := h.itemService.Create(userID, req)
	if err != nil {
		SendError(c, http.StatusBadRequest, "CREATE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusCreated, item, "Item created successfully")
}

func (h *ItemHandler) GetByID(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid item ID format", nil)
		return
	}

	detail, err := h.itemService.GetByID(userID, id)
	if err != nil {
		SendError(c, http.StatusNotFound, "ITEM_NOT_FOUND", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, detail, "")
}

func (h *ItemHandler) Update(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid item ID format", nil)
		return
	}

	var req service.UpdateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	item, err := h.itemService.Update(userID, id, req)
	if err != nil {
		SendError(c, http.StatusBadRequest, "UPDATE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, item, "Item updated successfully")
}

func (h *ItemHandler) Move(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid item ID format", nil)
		return
	}

	var req MoveItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	err = h.itemService.Move(userID, id, req.TargetBoxID)
	if err != nil {
		SendError(c, http.StatusBadRequest, "MOVE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, gin.H{"id": id, "targetBoxId": req.TargetBoxID}, "Item moved successfully")
}

func (h *ItemHandler) UpdateQuantity(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid item ID format", nil)
		return
	}

	var req UpdateQuantityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	err = h.itemService.UpdateQuantity(userID, id, req.Quantity)
	if err != nil {
		SendError(c, http.StatusBadRequest, "UPDATE_QUANTITY_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, gin.H{"id": id, "quantity": req.Quantity}, "Quantity updated successfully")
}

func (h *ItemHandler) Delete(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid item ID format", nil)
		return
	}

	err = h.itemService.Delete(userID, id)
	if err != nil {
		SendError(c, http.StatusBadRequest, "DELETE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, gin.H{"deleted": id}, "Item deleted successfully")
}
