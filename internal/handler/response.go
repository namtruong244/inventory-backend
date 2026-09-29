package handler

import "inventory_backend/internal/response"

type Meta = response.Meta
type ErrorDetail = response.ErrorDetail
type ErrorPayload = response.ErrorPayload
type ResponseEnvelope = response.ResponseEnvelope

var (
	SendSuccess         = response.SendSuccess
	SendSuccessWithMeta = response.SendSuccessWithMeta
	SendError           = response.SendError
)
