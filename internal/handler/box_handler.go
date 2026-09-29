package handler

import (
	"net/http"
	"strconv"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type BoxHandler struct {
	boxService service.BoxService
}

func NewBoxHandler(boxService service.BoxService) *BoxHandler {
	return &BoxHandler{boxService: boxService}
}

type MoveBoxRequest struct {
	NewParentID *uuid.UUID `json:"newParentId"`
}

func (h *BoxHandler) List(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	parentID := c.Query("parentId")
	treeStr := c.Query("tree")
	tree := treeStr == "true"

	data, err := h.boxService.List(userID, parentID, tree)
	if err != nil {
		SendError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, data, "")
}

func (h *BoxHandler) Create(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	var req service.CreateBoxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	box, err := h.boxService.Create(userID, req)
	if err != nil {
		SendError(c, http.StatusBadRequest, "CREATE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusCreated, box, "Storage space created successfully")
}

func (h *BoxHandler) GetByID(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid box ID format", nil)
		return
	}

	detail, err := h.boxService.GetByID(userID, id)
	if err != nil {
		SendError(c, http.StatusNotFound, "BOX_NOT_FOUND", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, detail, "")
}

func (h *BoxHandler) Update(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid box ID format", nil)
		return
	}

	var req service.UpdateBoxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	box, err := h.boxService.Update(userID, id, req)
	if err != nil {
		SendError(c, http.StatusBadRequest, "UPDATE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, box, "Box updated successfully")
}

func (h *BoxHandler) Move(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid box ID format", nil)
		return
	}

	var req MoveBoxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	err = h.boxService.Move(userID, id, req.NewParentID)
	if err != nil {
		SendError(c, http.StatusBadRequest, "MOVE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, gin.H{"id": id, "newParentId": req.NewParentID}, "Box moved successfully")
}

func (h *BoxHandler) Delete(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_ID", "Invalid box ID format", nil)
		return
	}

	cascadeStr := c.DefaultQuery("cascade", "false")
	cascade, _ := strconv.ParseBool(cascadeStr)

	err = h.boxService.Delete(userID, id, cascade)
	if err != nil {
		SendError(c, http.StatusBadRequest, "DELETE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, gin.H{"deleted": id}, "Box deleted successfully")
}
