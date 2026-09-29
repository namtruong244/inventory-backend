package handler

import (
	"net/http"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
)

type MediaHandler struct {
	mediaService service.MediaService
}

func NewMediaHandler(mediaService service.MediaService) *MediaHandler {
	return &MediaHandler{mediaService: mediaService}
}

func (h *MediaHandler) Upload(c *gin.Context) {
	_, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_FILE", "A binary file is required in 'file' form field", nil)
		return
	}

	folder := c.DefaultPostForm("folder", "items")

	res, err := h.mediaService.Upload(file, folder)
	if err != nil {
		SendError(c, http.StatusBadRequest, "UPLOAD_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, res, "File uploaded successfully")
}
