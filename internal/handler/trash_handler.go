package handler

import (
	"net/http"
	"strconv"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TrashHandler struct {
	trashService service.TrashService
}

func NewTrashHandler(s service.TrashService) *TrashHandler {
	return &TrashHandler{trashService: s}
}

// byID adapts a service method that takes (userID, id) into a Gin handler.
func (h *TrashHandler) byID(code, msg string, fn func(userID, id uuid.UUID) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetCurrentUserID(c)
		if !ok {
			SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid ID format", nil)
			return
		}
		if err := fn(userID, id); err != nil {
			fail(c, code, err)
			return
		}
		SendSuccess(c, http.StatusOK, gin.H{"id": id}, msg)
	}
}

func (h *TrashHandler) List(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}
	res, err := h.trashService.List(userID)
	if err != nil {
		fail(c, "TRASH_LIST_FAILED", err)
		return
	}
	SendSuccess(c, http.StatusOK, res, "Trash retrieved successfully")
}

func (h *TrashHandler) RestoreItem(c *gin.Context) {
	h.byID("RESTORE_FAILED", "Item restored successfully", h.trashService.RestoreItem)(c)
}
func (h *TrashHandler) RestoreBox(c *gin.Context) {
	h.byID("RESTORE_FAILED", "Box restored successfully", h.trashService.RestoreBox)(c)
}
func (h *TrashHandler) PurgeItem(c *gin.Context) {
	h.byID("PURGE_FAILED", "Item permanently deleted", h.trashService.PurgeItem)(c)
}
func (h *TrashHandler) PurgeBox(c *gin.Context) {
	h.byID("PURGE_FAILED", "Box permanently deleted", h.trashService.PurgeBox)(c)
}

func (h *TrashHandler) Empty(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}
	// A malformed value must not silently become 0, which empties the whole trash.
	days, err := strconv.Atoi(c.DefaultQuery("olderThanDays", "0"))
	if err != nil || days < 0 {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", "olderThanDays must be a non-negative integer", nil)
		return
	}
	n, err := h.trashService.Empty(userID, days)
	if err != nil {
		fail(c, "PURGE_FAILED", err)
		return
	}
	SendSuccess(c, http.StatusOK, gin.H{"removed": n}, "Trash emptied")
}
