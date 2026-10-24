package http

import (
	"github.com/gin-gonic/gin"
	"github.com/swaggest/swgui/v4cdn"
	"net/http"
)

func RegisterSwagger(router gin.IRouter) error {
	spec, err := GetSwagger()
	if err != nil {
		return err
	}
	handler := v4cdn.NewHandler("HTTP API template", "/api/v1/docs.json", "/")
	router.GET("/documentation/*any", gin.WrapH(handler))
	router.GET("/docs.json", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, spec)
	})
	return nil
}
