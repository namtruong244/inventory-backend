package handler

import (
	"log"
	"net/http"

	"inventory_backend/internal/apperr"

	"github.com/gin-gonic/gin"
)

// fail maps a service error to an HTTP response. Typed business errors keep
// their message; anything else is logged and hidden behind a generic 500 so
// database details never reach the client. invalidCode is the error code used
// for rule violations (HTTP 400).
func fail(c *gin.Context, invalidCode string, err error) {
	err = apperr.NotFoundOr(err, "resource not found")
	if e, ok := apperr.As(err); ok {
		switch e.Kind {
		case apperr.KindNotFound:
			SendError(c, http.StatusNotFound, "NOT_FOUND", e.Msg, nil)
		case apperr.KindConflict:
			SendError(c, http.StatusConflict, "CONFLICT", e.Msg, nil)
		default:
			SendError(c, http.StatusBadRequest, invalidCode, e.Msg, nil)
		}
		return
	}
	log.Printf("internal error on %s %s: %v", c.Request.Method, c.FullPath(), err)
	SendError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
}
