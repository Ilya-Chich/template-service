package http

import (
	"github.com/gin-gonic/gin"
	"http-api-template/internal/adapter/http/view"
	"net/http"
)

func (h *handler) health(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, view.HealthResponse{Status: view.N200})
}
