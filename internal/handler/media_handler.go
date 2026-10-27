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
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	// Bound the request body (file + small multipart overhead) before parsing the form
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxUploadSize+(1<<20))

	file, err := c.FormFile("file")
	if err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_FILE", "A binary file is required in 'file' form field", nil)
		return
	}

	folder := c.DefaultPostForm("folder", "items")

	res, err := h.mediaService.Upload(userID, file, folder)
	if err != nil {
		// Validation messages are for the user; storage errors stay in the log.
		fail(c, "UPLOAD_FAILED", err)
		return
	}

	SendSuccess(c, http.StatusOK, res, "File uploaded successfully")
}
