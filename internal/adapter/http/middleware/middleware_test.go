package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	logadapter "http-api-template/internal/adapter/logger"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/requestid"
	portmocks "http-api-template/internal/mocks/port"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
)

const (
	fixtureID    = "550e8400-e29b-41d4-a716-446655440000"
	providedID   = "client-123"
	privateValue = "private-sensitive-value"
)

func TestRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name      string
		headers   []string
		want      string
		generated int
	}{
		{"accepted", []string{providedID}, providedID, 0},
		{"maximum", []string{strings.Repeat("a", 128)}, strings.Repeat("a", 128), 0},
		{"missing", nil, fixtureID, 1},
		{"empty", []string{""}, fixtureID, 1},
		{"oversized", []string{strings.Repeat("a", 129)}, fixtureID, 1},
		{"invalid characters", []string{"invalid id"}, fixtureID, 1},
		{"non-ASCII", []string{"ž"}, fixtureID, 1},
		{"duplicate", []string{providedID, providedID}, fixtureID, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := portmocks.NewMockIDGenerator(t)
			if tc.generated != 0 {
				ids.EXPECT().NewID().Return(fixtureID, nil).Once()
			}
			var logs bytes.Buffer
			router := gin.New()
			router.Use(RequestID(ids), AccessLog(logadapter.New(&logs, ids)))
			router.GET("/test", func(ctx *gin.Context) {
				if requestid.ID(ctx.Request.Context()) != tc.want ||
					ctx.Writer.Header().Get(RequestIDHeader) != tc.want {
					t.Error("correlation was not set before the handler")
				}
				ctx.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodGet, "/test?password="+privateValue, nil)
			req.Header.Set("Authorization", "Bearer "+privateValue)
			for _, value := range tc.headers {
				req.Header.Add(RequestIDHeader, value)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusOK || response.Header().Get(RequestIDHeader) != tc.want {
				t.Fatalf("status=%d ID=%q", response.Code, response.Header().Get(RequestIDHeader))
			}
			var record map[string]any
			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record["request_id"] != tc.want || record["status"] != float64(http.StatusOK) ||
				strings.Contains(logs.String(), privateValue) {
				t.Fatal("request log correlation, status, or privacy failed")
			}
		})
	}
}

func TestJWTMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		header string
		err    error
		want   int
		calls  int
	}{
		{"missing", "", nil, http.StatusUnauthorized, 0},
		{"wrong scheme", "Basic value", nil, http.StatusUnauthorized, 0},
		{"missing token", "Bearer ", nil, http.StatusUnauthorized, 0},
		{"extra fields", "Bearer one two", nil, http.StatusUnauthorized, 0},
		{"invalid token", "Bearer test-token", domain.ErrUnauthorized, http.StatusUnauthorized, 1},
		{"valid token", "Bearer test-token", nil, http.StatusNoContent, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verifier := portmocks.NewMockTokenVerifier(t)
			if tc.calls != 0 {
				verifier.EXPECT().
					Verify(mock.MatchedBy(func(ctx context.Context) bool { return requestid.ID(ctx) == providedID }), "test-token").
					Return(tc.err).
					Once()
			}
			router := gin.New()
			router.Use(RequestID(portmocks.NewMockIDGenerator(t)), JWT(verifier))
			called := false
			router.POST("/write", func(ctx *gin.Context) {
				called = true
				ctx.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodPost, "/write", nil)
			req.Header.Set(RequestIDHeader, providedID)
			req.Header.Set("Authorization", tc.header)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.want || called != (tc.want == http.StatusNoContent) {
				t.Fatalf("status=%d handler called=%v", response.Code, called)
			}
		})
	}
}

func TestRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := portmocks.NewMockIDGenerator(t)
	var output bytes.Buffer
	log := logadapter.New(&output, ids)
	router := gin.New()
	router.Use(RequestID(ids), AccessLog(log), Recovery(log))
	router.GET("/panic", func(_ *gin.Context) { panic(privateValue) })
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set(RequestIDHeader, providedID)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusInternalServerError || strings.Contains(output.String(), privateValue) {
		t.Fatal("recovery did not protect panic details")
	}
	decoder := json.NewDecoder(&output)
	for range 2 {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record["request_id"] != providedID {
			t.Fatal("recovery/access log lost correlation")
		}
	}
}

func TestRequestIDGenerationFailure(t *testing.T) {
	router := gin.New()
	ids := portmocks.NewMockIDGenerator(t)
	ids.EXPECT().NewID().Return("", errors.New("entropy unavailable")).Once()
	router.Use(RequestID(ids))
	router.GET("/test", func(_ *gin.Context) { t.Error("handler must not run") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/test", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", response.Code)
	}
}
