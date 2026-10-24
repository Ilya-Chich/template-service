package middleware

import (
	"github.com/gin-gonic/gin"
	"http-api-template/internal/core/port"
	"net/http"
	"strings"
)

func JWT(verifier port.TokenVerifier) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		fields := strings.Fields(ctx.GetHeader("Authorization"))
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if err := verifier.Verify(ctx.Request.Context(), fields[1]); err != nil {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ctx.Next()
	}
}
