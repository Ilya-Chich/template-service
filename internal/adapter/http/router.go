package http

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"http-api-template/internal/adapter/http/middleware"
	"http-api-template/internal/core/port"
)

type handler struct {
	dummies      port.DummyService
	auth         port.AuthService
	logger       port.Logger
	createSchema *openapi3.Schema
	patchSchema  *openapi3.Schema
	loginSchema  *openapi3.Schema
}

func NewRouter(dummies port.DummyService, auth port.AuthService, verifier port.TokenVerifier, logger port.Logger, ids port.IDGenerator) (*gin.Engine, error) {
	spec, err := GetSwagger()
	if err != nil {
		return nil, err
	}
	h := &handler{
		dummies: dummies, auth: auth, logger: logger,
		createSchema: spec.Paths["/api/v1/dummies"].Post.RequestBody.Value.Content["application/json"].Schema.Value,
		patchSchema:  spec.Paths["/api/v1/dummies/{id}"].Patch.RequestBody.Value.Content["application/json"].Schema.Value,
		loginSchema:  spec.Paths["/api/v1/auth/login"].Post.RequestBody.Value.Content["application/json"].Schema.Value,
	}
	router := gin.New()
	router.Use(middleware.RequestID(ids), middleware.AccessLog(logger), middleware.Recovery(logger))
	v1 := router.Group("/api/v1")
	if err := RegisterSwagger(v1); err != nil {
		return nil, err
	}
	v1.GET("/health", h.health)
	v1.POST("/auth/login", h.login)
	v1.GET("/dummies/:id", h.get)
	writes := v1.Group("")
	writes.Use(middleware.JWT(verifier))
	writes.POST("/dummies", h.create)
	writes.PATCH("/dummies/:id", h.patch)
	writes.DELETE("/dummies/:id", h.remove)
	return router, nil
}
