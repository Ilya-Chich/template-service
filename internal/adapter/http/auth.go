package http

import (
	"github.com/gin-gonic/gin"
	"http-api-template/internal/adapter/http/view"
	"http-api-template/internal/core/domain"
	"net/http"
	"net/mail"
)

func (h *handler) login(ctx *gin.Context) {
	var req view.AuthRequest
	if !decodeBody(ctx, &req, h.loginSchema) {
		return
	}
	email := string(req.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || req.Password == "" {
		h.fail(ctx, domain.ErrInvalid, false)
		return
	}
	token, err := h.auth.Login(ctx.Request.Context(), email, req.Password)
	if err != nil {
		h.fail(ctx, err, false)
		return
	}
	ctx.JSON(http.StatusOK, view.AuthResponse{
		AccessToken: token.AccessToken,
		TokenType:   view.AuthResponseTokenType(token.TokenType),
		ExpiresIn:   view.AuthResponseExpiresIn(token.ExpiresIn),
	})
}
