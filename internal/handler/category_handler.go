package handler

import (
	"net/http"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
)

type CategoryHandler struct {
	categoryService service.CategoryService
}

func NewCategoryHandler(categoryService service.CategoryService) *CategoryHandler {
	return &CategoryHandler{categoryService: categoryService}
}

type CreateCategoryRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *CategoryHandler) List(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	categories, err := h.categoryService.List(userID)
	if err != nil {
		SendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, categories, "")
}

func (h *CategoryHandler) Create(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	cat, err := h.categoryService.Create(userID, req.Name)
	if err != nil {
		SendError(c, http.StatusBadRequest, "CREATE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusCreated, cat, "Category created successfully")
}

func (h *CategoryHandler) Delete(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	name := c.Param("name")
	if name == "" {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", "Category name is required", nil)
		return
	}

	err := h.categoryService.Delete(userID, name)
	if err != nil {
		SendError(c, http.StatusBadRequest, "DELETE_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, gin.H{"deleted": name}, "Category deleted successfully")
}
