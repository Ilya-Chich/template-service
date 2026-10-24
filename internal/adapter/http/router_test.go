package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authadapter "http-api-template/internal/adapter/auth"
	"http-api-template/internal/adapter/http/middleware"
	logadapter "http-api-template/internal/adapter/logger"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/requestid"
	portmocks "http-api-template/internal/mocks/port"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
)

const (
	fixtureDocsPath  = "/api/v1/docs.json"
	operationPatch   = "patch"
	operationDelete  = "delete"
	nullNameBody     = `{"name":null}`
	createBody       = `{"name":"Example"}`
	clearNoteBody    = `{"note":""}`
	loginBody        = `{"email":"user@example.com","password":"test-password"}`
	fixtureDummyID   = "550e8400-e29b-41d4-a716-446655440000"
	fixtureDummyName = "Example"
	dummiesPath      = "/api/v1/dummies"
	dummyPath        = dummiesPath + "/" + fixtureDummyID
	loginPath        = "/api/v1/auth/login"
	fixtureRequestID = "http-test-request"
	fixtureEmail     = "user@example.com"
)

type httpFixture struct {
	router  *gin.Engine
	dummies *portmocks.MockDummyService
	auth    *portmocks.MockAuthService
	token   string
	logs    *bytes.Buffer
}

func newHTTPFixture(t *testing.T) *httpFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tokens, err := authadapter.NewJWT("http-fixture-signing-key")
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokens.Issue(context.Background(), fixtureEmail)
	if err != nil {
		t.Fatal(err)
	}
	dummies := portmocks.NewMockDummyService(t)
	auth := portmocks.NewMockAuthService(t)
	logs := &bytes.Buffer{}
	router, err := NewRouter(
		dummies,
		auth,
		tokens,
		logadapter.New(logs, portmocks.NewMockIDGenerator(t)),
		portmocks.NewMockIDGenerator(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &httpFixture{router: router, dummies: dummies, auth: auth, token: token.AccessToken, logs: logs}
}

func (f *httpFixture) request(method, path, body string, protected bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.RequestIDHeader, fixtureRequestID)
	if protected {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, req)
	return response
}

func TestHealthAndDocumentation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := portmocks.NewMockIDGenerator(t)
	ids.EXPECT().NewID().Return("generated-request-id", nil).Times(4)
	router, err := NewRouter(nil, nil, nil, logadapter.New(&bytes.Buffer{}, ids), ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/health", "/api/v1/health?format=prometheus", "/api/v1/documentation/", fixtureDocsPath} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK || response.Header().Get(middleware.RequestIDHeader) == "" {
				t.Fatalf("status=%d", response.Code)
			}
			if strings.HasPrefix(path, "/api/v1/health") &&
				(response.Body.String() != `{"status":200}` || !strings.Contains(response.Header().Get("Content-Type"), "application/json")) {
				t.Fatal("health must always return JSON without consulting services")
			}
		})
	}
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "companies") {
			t.Fatal("legacy resource routes are still active")
		}
	}
}

func TestGetDummy(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"found", nil, http.StatusOK},
		{"missing", fmt.Errorf("lookup: %w", domain.ErrNotFound), http.StatusNotFound},
		{"unavailable", errors.New("private-database-error"), http.StatusInternalServerError},
		{"invalid", fmt.Errorf("validation: %w", domain.ErrInvalid), http.StatusBadRequest},
		{"unauthorized", fmt.Errorf("auth: %w", domain.ErrUnauthorized), http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHTTPFixture(t)
			f.dummies.EXPECT().
				Get(httpRequestContext(), fixtureDummyID).
				Return(domain.Dummy{ID: fixtureDummyID, Name: fixtureDummyName}, tc.err).
				Once()
			response := f.request(http.MethodGet, dummyPath, "", false)
			if response.Code != tc.want {
				t.Fatalf("status=%d", response.Code)
			}
			if tc.want == http.StatusOK {
				var value map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
					t.Fatal(err)
				}
				if len(value) != 3 || value["id"] != fixtureDummyID || value["name"] != fixtureDummyName ||
					value["note"] != "" {
					t.Fatalf("body=%s", response.Body.String())
				}
			} else if response.Body.Len() != 0 {
				t.Fatal("errors must have empty bodies")
			}
			if strings.Contains(f.logs.String(), "private-database-error") {
				t.Fatal("internal error details were logged")
			}
		})
	}
}

func TestAcceptedWrites(t *testing.T) {
	for _, tc := range []struct {
		method    string
		path      string
		body      string
		operation string
	}{
		{http.MethodPost, dummiesPath, `{"name":" Example ","note":"note"}`, "create"},
		{http.MethodPatch, dummyPath, clearNoteBody, operationPatch},
		{http.MethodDelete, dummyPath, "", operationDelete},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			f := newHTTPFixture(t)
			empty := ""
			switch tc.method {
			case http.MethodPost:
				f.dummies.EXPECT().Create(httpRequestContext(), " Example ", "note").Return(fixtureDummyID, nil).Once()
			case http.MethodPatch:
				f.dummies.EXPECT().
					Patch(httpRequestContext(), fixtureDummyID, domain.Patch{Note: &empty}).
					Return(nil).
					Once()
			case http.MethodDelete:
				f.dummies.EXPECT().Delete(httpRequestContext(), fixtureDummyID).Return(nil).Once()
			}
			response := f.request(tc.method, tc.path, tc.body, true)
			if response.Code != http.StatusAccepted {
				t.Fatalf("status=%d", response.Code)
			}
			switch tc.method {
			case http.MethodPost:
				var value map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
					t.Fatal(err)
				}
				if len(value) != 1 || value["id"] != fixtureDummyID {
					t.Fatal("create response or mapped input was incorrect")
				}
			case http.MethodPatch:
				if response.Body.Len() != 0 {
					t.Fatal("note clearing or omitted-field semantics were lost")
				}
			case http.MethodDelete:
				if response.Body.Len() != 0 {
					t.Fatal("delete acceptance must have an empty body")
				}
			}
		})
	}
	t.Run("omitted create note", func(t *testing.T) {
		f := newHTTPFixture(t)
		f.dummies.EXPECT().Create(httpRequestContext(), fixtureDummyName, "").Return(fixtureDummyID, nil).Once()
		if response := f.request(http.MethodPost, dummiesPath, createBody, true); response.Code != http.StatusAccepted {
			t.Fatal("omitted note must become an empty string")
		}
	})
	t.Run("name-only patch", func(t *testing.T) {
		f := newHTTPFixture(t)
		name := "Renamed"
		f.dummies.EXPECT().Patch(httpRequestContext(), fixtureDummyID, domain.Patch{Name: &name}).Return(nil).Once()
		response := f.request(http.MethodPatch, dummyPath, `{"name":"Renamed"}`, true)
		if response.Code != http.StatusAccepted {
			t.Fatal("name-only patch was mapped incorrectly")
		}
	})
}

func TestInvalidRequestsDoNotCallService(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"missing name", http.MethodPost, dummiesPath, `{}`},
		{"null name", http.MethodPost, dummiesPath, nullNameBody},
		{"blank name", http.MethodPost, dummiesPath, `{"name":"  "}`},
		{"null note", http.MethodPost, dummiesPath, `{"name":"Example","note":null}`},
		{"wrong note type", http.MethodPost, dummiesPath, `{"name":"Example","note":1}`},
		{"malformed", http.MethodPost, dummiesPath, `{`},
		{"trailing JSON", http.MethodPost, dummiesPath, `{"name":"Example"}{}`},
		{"null body", http.MethodPost, dummiesPath, `null`},
		{"array body", http.MethodPost, dummiesPath, `[]`},
		{"empty patch", http.MethodPatch, dummyPath, `{}`},
		{"unknown-only patch", http.MethodPatch, dummyPath, `{"other":"value"}`},
		{"null patch name", http.MethodPatch, dummyPath, nullNameBody},
		{"null patch note", http.MethodPatch, dummyPath, `{"note":null}`},
		{"blank patch name", http.MethodPatch, dummyPath, `{"name":" "}`},
		{"wrong patch type", http.MethodPatch, dummyPath, `{"note":false}`},
		{"invalid GET UUID", http.MethodGet, dummiesPath + "/invalid", ""},
		{"invalid PATCH UUID", http.MethodPatch, dummiesPath + "/invalid", clearNoteBody},
		{"invalid DELETE UUID", http.MethodDelete, dummiesPath + "/invalid", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHTTPFixture(t)
			response := f.request(tc.method, tc.path, tc.body, true)
			if response.Code != http.StatusBadRequest || response.Body.Len() != 0 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestWriteErrors(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		for _, tc := range []struct {
			name string
			err  error
			want int
		}{
			{"publisher failure", errors.New("publication failed"), http.StatusInternalServerError},
			{"wrapped invalid", fmt.Errorf("service: %w", domain.ErrInvalid), http.StatusBadRequest},
			{"wrapped unauthorized", fmt.Errorf("service: %w", domain.ErrUnauthorized), http.StatusUnauthorized},
			{"unexpected missing", domain.ErrNotFound, http.StatusInternalServerError},
			{"unexpected conflict", domain.ErrConflict, http.StatusInternalServerError},
		} {
			t.Run(method+" "+tc.name, func(t *testing.T) {
				f := newHTTPFixture(t)
				empty := ""
				switch method {
				case http.MethodPost:
					f.dummies.EXPECT().Create(httpRequestContext(), fixtureDummyName, "").Return("", tc.err).Once()
				case http.MethodPatch:
					f.dummies.EXPECT().
						Patch(httpRequestContext(), fixtureDummyID, domain.Patch{Note: &empty}).
						Return(tc.err).
						Once()
				case http.MethodDelete:
					f.dummies.EXPECT().Delete(httpRequestContext(), fixtureDummyID).Return(tc.err).Once()
				}
				path, body := dummyPath, clearNoteBody
				if method == http.MethodPost {
					path, body = dummiesPath, createBody
				}
				response := f.request(method, path, body, true)
				want := tc.want
				if method == http.MethodPost && errors.Is(tc.err, domain.ErrConflict) {
					want = http.StatusConflict
				}
				if response.Code != want || response.Body.Len() != 0 {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			})
		}
	}
}

func TestProtectedWrites(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		for _, invalidToken := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s invalid=%v", method, invalidToken), func(t *testing.T) {
				f := newHTTPFixture(t)
				if invalidToken {
					f.token = "invalid-token"
				}
				path := dummyPath
				if method == http.MethodPost {
					path = dummiesPath
				}
				response := f.request(method, path, "{}", invalidToken)
				if response.Code != http.StatusUnauthorized || response.Body.Len() != 0 {
					t.Fatalf("status=%d", response.Code)
				}
			})
		}
	}
}

func TestLogin(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		err   error
		want  int
		calls int
	}{
		{"valid", loginBody, nil, http.StatusOK, 1},
		{"invalid email", `{"email":"invalid","password":"test-password"}`, nil, http.StatusBadRequest, 0},
		{"missing password", `{"email":"user@example.com"}`, nil, http.StatusBadRequest, 0},
		{"empty password", `{"email":"user@example.com","password":""}`, nil, http.StatusBadRequest, 0},
		{"null email", `{"email":null,"password":"test-password"}`, nil, http.StatusBadRequest, 0},
		{"issuer failure", loginBody, errors.New("issuer failed"), http.StatusInternalServerError, 1},
		{"unauthorized", loginBody, domain.ErrUnauthorized, http.StatusUnauthorized, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHTTPFixture(t)
			if tc.calls != 0 {
				f.auth.EXPECT().
					Login(httpRequestContext(), fixtureEmail, "test-password").
					Return(domain.Token{AccessToken: f.token, TokenType: "Bearer", ExpiresIn: 3600}, tc.err).
					Once()
			}
			response := f.request(http.MethodPost, loginPath, tc.body, false)
			if response.Code != tc.want {
				t.Fatalf("status=%d", response.Code)
			}
			if tc.want == http.StatusOK {
				var token map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &token); err != nil {
					t.Fatal(err)
				}
				if token["access_token"] != f.token || token["token_type"] != "Bearer" ||
					token["expires_in"] != float64(3600) {
					t.Fatal("login response or correlation was lost")
				}
			} else if response.Body.Len() != 0 {
				t.Fatal("login error must have an empty body")
			}
			if strings.Contains(f.logs.String(), "test-password") || strings.Contains(f.logs.String(), f.token) {
				t.Fatal("credentials were logged")
			}
		})
	}
}

func TestInvalidServiceResponsesAreInternalErrors(t *testing.T) {
	f := newHTTPFixture(t)
	f.dummies.EXPECT().Create(httpRequestContext(), fixtureDummyName, "").Return("invalid", nil).Once()
	if response := f.request(http.MethodPost, dummiesPath, createBody, true); response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", response.Code)
	}
	f.dummies.EXPECT().Get(httpRequestContext(), fixtureDummyID).Return(domain.Dummy{ID: fixtureDummyID}, nil).Once()
	if response := f.request(http.MethodGet, dummyPath, "", false); response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestCreateWaitsForPublication(t *testing.T) {
	f := newHTTPFixture(t)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	f.dummies.EXPECT().
		Create(httpRequestContext(), fixtureDummyName, "").
		RunAndReturn(func(_ context.Context, _, _ string) (string, error) {
			close(started)
			<-release
			return fixtureDummyID, nil
		}).
		Once()
	var response *httptest.ResponseRecorder
	go func() {
		response = f.request(http.MethodPost, dummiesPath, createBody, true)
		close(done)
	}()
	<-started
	select {
	case <-done:
		t.Error("create returned before publication completed")
	default:
	}
	close(release)
	<-done
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d", response.Code)
	}
}

// Match correlation without coupling HTTP tests to Gin's context implementation.
func httpRequestContext() interface{} {
	return mock.MatchedBy(func(ctx context.Context) bool { return requestid.ID(ctx) == fixtureRequestID })
}

func TestServiceDeadlineErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name    string
		expired bool
		err     error
		want    int
	}{
		{"expired request", true, fmt.Errorf("query: %w", context.DeadlineExceeded), http.StatusGatewayTimeout},
		{"dependency deadline only", false, context.DeadlineExceeded, http.StatusInternalServerError},
		{"unrelated failure after deadline", true, errors.New("dependency failure"), http.StatusInternalServerError},
		{"cancellation", false, context.Canceled, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHTTPFixture(t)
			f.dummies.EXPECT().Get(httpRequestContext(), fixtureDummyID).Return(domain.Dummy{}, tc.err).Once()
			request := httptest.NewRequest(http.MethodGet, dummyPath, nil)
			request.Header.Set(middleware.RequestIDHeader, fixtureRequestID)
			if tc.expired {
				ctx, cancel := context.WithDeadline(request.Context(), time.Now().Add(-time.Second))
				defer cancel()
				request = request.WithContext(ctx)
			}
			response := httptest.NewRecorder()
			f.router.ServeHTTP(response, request)
			if response.Code != tc.want || response.Body.Len() != 0 {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
}
