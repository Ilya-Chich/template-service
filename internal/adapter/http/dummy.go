package http

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"http-api-template/internal/adapter/http/view"
	"http-api-template/internal/core/domain"
	"net/http"
)

func (h *handler) get(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := domain.ValidateID(id); err != nil {
		h.fail(ctx, err, true)
		return
	}
	dummy, err := h.dummies.Get(ctx.Request.Context(), id)
	if err != nil {
		h.fail(ctx, err, true)
		return
	}
	if err := dummy.Validate(); err != nil {
		h.invalidResponse(ctx)
		return
	}
	parsedID, err := uuid.Parse(dummy.ID)
	if err != nil {
		h.invalidResponse(ctx)
		return
	}
	ctx.JSON(http.StatusOK, view.GetDummyResponse{Id: parsedID, Name: dummy.Name, Note: dummy.Note})
}
func (h *handler) create(ctx *gin.Context) {
	var req view.CreateDummyRequest
	if !decodeBody(ctx, &req, h.createSchema) {
		return
	}
	if err := domain.ValidateName(req.Name); err != nil {
		h.fail(ctx, err, false)
		return
	}
	note := ""
	if req.Note != nil {
		note = *req.Note
	}
	id, err := h.dummies.Create(ctx.Request.Context(), req.Name, note)
	if errors.Is(err, domain.ErrConflict) {
		ctx.Status(http.StatusConflict)
		return
	}
	if err != nil {
		h.fail(ctx, err, false)
		return
	}
	if err := domain.ValidateID(id); err != nil {
		h.invalidResponse(ctx)
		return
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		h.invalidResponse(ctx)
		return
	}
	ctx.JSON(http.StatusAccepted, view.CreateDummyResponse{Id: parsedID})
}
func (h *handler) patch(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := domain.ValidateID(id); err != nil {
		h.fail(ctx, err, false)
		return
	}
	var req view.PatchDummyRequest
	if !decodeBody(ctx, &req, h.patchSchema) {
		return
	}
	patch := domain.Patch{Name: req.Name, Note: req.Note}
	if err := patch.Validate(); err != nil {
		h.fail(ctx, err, false)
		return
	}
	if err := h.dummies.Patch(ctx.Request.Context(), id, patch); err != nil {
		h.fail(ctx, err, false)
		return
	}
	ctx.Status(http.StatusAccepted)
}
func (h *handler) remove(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := domain.ValidateID(id); err != nil {
		h.fail(ctx, err, false)
		return
	}
	if err := h.dummies.Delete(ctx.Request.Context(), id); err != nil {
		h.fail(ctx, err, false)
		return
	}
	ctx.Status(http.StatusAccepted)
}
