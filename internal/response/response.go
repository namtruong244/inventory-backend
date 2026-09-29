package response

import (
	"github.com/gin-gonic/gin"
)

type Meta struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
}

type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type ErrorPayload struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details"`
}

type ResponseEnvelope struct {
	Success bool          `json:"success"`
	Data    interface{}   `json:"data,omitempty"`
	Message string        `json:"message,omitempty"`
	Meta    *Meta         `json:"meta,omitempty"`
	Error   *ErrorPayload `json:"error,omitempty"`
}

func SendSuccess(c *gin.Context, httpStatus int, data interface{}, message string) {
	c.JSON(httpStatus, ResponseEnvelope{
		Success: true,
		Data:    data,
		Message: message,
	})
}

func SendSuccessWithMeta(c *gin.Context, httpStatus int, data interface{}, meta *Meta, message string) {
	c.JSON(httpStatus, ResponseEnvelope{
		Success: true,
		Data:    data,
		Meta:    meta,
		Message: message,
	})
}

func SendError(c *gin.Context, httpStatus int, code string, message string, details []ErrorDetail) {
	if details == nil {
		details = make([]ErrorDetail, 0)
	}
	c.JSON(httpStatus, ResponseEnvelope{
		Success: false,
		Error: &ErrorPayload{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
