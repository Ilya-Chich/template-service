package http

import (
	"net/http"

	"http-api-template/internal/adapter/http/view"

	"github.com/gin-gonic/gin"
)

func (h *handler) health(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, view.HealthResponse{Status: view.N200})
}
