package middleware

import (
	"github.com/gin-gonic/gin"
	"http-api-template/internal/core/port"
	"http-api-template/internal/core/requestid"
	"net/http"
	"time"
)

const RequestIDHeader = "X-Request-ID"

func RequestID(ids port.IDGenerator) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		id := ""
		if values := ctx.Request.Header.Values(RequestIDHeader); len(values) == 1 && requestid.Valid(values[0]) {
			id = values[0]
		}
		requestCtx, err := requestid.Ensure(requestid.WithID(ctx.Request.Context(), id), ids)
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		ctx.Request = ctx.Request.WithContext(requestCtx)
		ctx.Header(RequestIDHeader, requestid.ID(requestCtx))
		ctx.Next()
	}
}
func AccessLog(log port.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		started := time.Now()
		ctx.Next()
		log.Info(ctx.Request.Context(), "HTTP request", "method", ctx.Request.Method, "route", ctx.FullPath(), "status", ctx.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	}
}
func Recovery(log port.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		defer func() {
			if recover() != nil {
				log.Error(ctx.Request.Context(), "HTTP handler panic")
				ctx.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		ctx.Next()
	}
}
