package http

import (
	"encoding/json"
	"errors"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"http-api-template/internal/core/domain"
	"io"
	"net/http"
)

func decodeBody(ctx *gin.Context, target any, schema *openapi3.Schema) bool {
	decoder := json.NewDecoder(ctx.Request.Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		ctx.Status(http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		ctx.Status(http.StatusBadRequest)
		return false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		ctx.Status(http.StatusBadRequest)
		return false
	}
	if err := schema.VisitJSON(value); err != nil {
		ctx.Status(http.StatusBadRequest)
		return false
	}
	if err := json.Unmarshal(raw, target); err != nil {
		ctx.Status(http.StatusBadRequest)
		return false
	}
	return true
}
func (h *handler) fail(ctx *gin.Context, err error, allowNotFound bool) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		ctx.Status(http.StatusBadRequest)
	case errors.Is(err, domain.ErrUnauthorized):
		ctx.Status(http.StatusUnauthorized)
	case allowNotFound && errors.Is(err, domain.ErrNotFound):
		ctx.Status(http.StatusNotFound)
	default:
		h.logger.Error(ctx.Request.Context(), "HTTP service operation failed")
		ctx.Status(http.StatusInternalServerError)
	}
}
func (h *handler) invalidResponse(ctx *gin.Context) {
	h.logger.Error(ctx.Request.Context(), "HTTP service returned invalid response data")
	ctx.Status(http.StatusInternalServerError)
}
