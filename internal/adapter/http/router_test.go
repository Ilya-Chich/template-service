package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	authadapter "http-api-template/internal/adapter/auth"
	"http-api-template/internal/adapter/http/middleware"
	logadapter "http-api-template/internal/adapter/logger"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/port"
	"http-api-template/internal/core/requestid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const fixtureDocsPath = "/api/v1/docs.json"
const operationPatch = "patch"
const operationDelete = "delete"
const nullNameBody = `{"name":null}`
const createBody = `{"name":"Example"}`
const clearNoteBody = `{"note":""}`
const loginBody = `{"email":"user@example.com","password":"test-password"}`
const fixtureDummyID = "550e8400-e29b-41d4-a716-446655440000"
const fixtureDummyName = "Example"
const dummiesPath = "/api/v1/dummies"
const dummyPath = dummiesPath + "/" + fixtureDummyID
const loginPath = "/api/v1/auth/login"
const fixtureRequestID = "http-test-request"
const fixtureEmail = "user@example.com"

type fixtureIDs struct{}

func (fixtureIDs) NewID() (string, error) {
	return "generated-request-id", nil
}

type fakeDummies struct {
	calls      []string
	requestIDs []string
	err        error
	dummy      domain.Dummy
	createID   string
	name       string
	note       string
	id         string
	patch      domain.Patch
	createFn   func(context.Context, string, string) (string, error)
}

var _ port.DummyService = (*fakeDummies)(nil)

func (f *fakeDummies) record(ctx context.Context, operation string) {
	f.calls = append(f.calls, operation)
	f.requestIDs = append(f.requestIDs, requestid.ID(ctx))
}
func (f *fakeDummies) Get(ctx context.Context, id string) (domain.Dummy, error) {
	f.record(ctx, "get")
	f.id = id
	return f.dummy, f.err
}
func (f *fakeDummies) Create(ctx context.Context, name, note string) (string, error) {
	f.record(ctx, "create")
	f.name, f.note = name, note
	if f.createFn != nil {
		return f.createFn(ctx, name, note)
	}
	return f.createID, f.err
}
func (f *fakeDummies) Patch(ctx context.Context, id string, patch domain.Patch) error {
	f.record(ctx, operationPatch)
	f.id, f.patch = id, patch
	return f.err
}
func (f *fakeDummies) Delete(ctx context.Context, id string) error {
	f.record(ctx, operationDelete)
	f.id = id
	return f.err
}

type fakeAuth struct {
	calls     int
	email     string
	password  string
	requestID string
	token     domain.Token
	err       error
}

var _ port.AuthService = (*fakeAuth)(nil)

func (f *fakeAuth) Login(ctx context.Context, email, password string) (domain.Token, error) {
	f.calls++
	f.email, f.password, f.requestID = email, password, requestid.ID(ctx)
	return f.token, f.err
}

type httpFixture struct {
	router  *gin.Engine
	dummies *fakeDummies
	auth    *fakeAuth
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
	dummies := &fakeDummies{dummy: domain.Dummy{ID: fixtureDummyID, Name: fixtureDummyName}, createID: fixtureDummyID}
	auth := &fakeAuth{token: token}
	logs := &bytes.Buffer{}
	router, err := NewRouter(dummies, auth, tokens, logadapter.New(logs, fixtureIDs{}), fixtureIDs{})
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
	router, err := NewRouter(nil, nil, nil, logadapter.New(&bytes.Buffer{}, fixtureIDs{}), fixtureIDs{})
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
			if strings.HasPrefix(path, "/api/v1/health") && (response.Body.String() != `{"status":200}` || !strings.Contains(response.Header().Get("Content-Type"), "application/json")) {
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
			f.dummies.err = tc.err
			response := f.request(http.MethodGet, dummyPath, "", false)
			if response.Code != tc.want || len(f.dummies.calls) != 1 || f.dummies.id != fixtureDummyID || f.dummies.requestIDs[0] != fixtureRequestID {
				t.Fatalf("status=%d calls=%v", response.Code, f.dummies.calls)
			}
			if tc.want == http.StatusOK {
				var value map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
					t.Fatal(err)
				}
				if len(value) != 3 || value["id"] != fixtureDummyID || value["name"] != fixtureDummyName || value["note"] != "" {
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
			response := f.request(tc.method, tc.path, tc.body, true)
			if response.Code != http.StatusAccepted || len(f.dummies.calls) != 1 || f.dummies.calls[0] != tc.operation || f.dummies.requestIDs[0] != fixtureRequestID {
				t.Fatalf("status=%d calls=%v", response.Code, f.dummies.calls)
			}
			switch tc.method {
			case http.MethodPost:
				var value map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
					t.Fatal(err)
				}
				if len(value) != 1 || value["id"] != fixtureDummyID || f.dummies.name != " Example " || f.dummies.note != "note" {
					t.Fatal("create response or mapped input was incorrect")
				}
			case http.MethodPatch:
				if f.dummies.patch.Note == nil || *f.dummies.patch.Note != "" || f.dummies.patch.Name != nil || response.Body.Len() != 0 {
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
		if response := f.request(http.MethodPost, dummiesPath, createBody, true); response.Code != http.StatusAccepted || f.dummies.note != "" {
			t.Fatal("omitted note must become an empty string")
		}
	})
	t.Run("name-only patch", func(t *testing.T) {
		f := newHTTPFixture(t)
		response := f.request(http.MethodPatch, dummyPath, `{"name":"Renamed"}`, true)
		if response.Code != http.StatusAccepted || f.dummies.patch.Name == nil || *f.dummies.patch.Name != "Renamed" || f.dummies.patch.Note != nil {
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
			if response.Code != http.StatusBadRequest || response.Body.Len() != 0 || len(f.dummies.calls) != 0 {
				t.Fatalf("status=%d calls=%v body=%s", response.Code, f.dummies.calls, response.Body.String())
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
				f.dummies.err = tc.err
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
				if response.Code != http.StatusUnauthorized || response.Body.Len() != 0 || len(f.dummies.calls) != 0 {
					t.Fatalf("status=%d calls=%v", response.Code, f.dummies.calls)
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
			f.auth.err = tc.err
			response := f.request(http.MethodPost, loginPath, tc.body, false)
			if response.Code != tc.want || f.auth.calls != tc.calls {
				t.Fatalf("status=%d auth calls=%d", response.Code, f.auth.calls)
			}
			if tc.want == http.StatusOK {
				var token map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &token); err != nil {
					t.Fatal(err)
				}
				if token["access_token"] != f.token || token["token_type"] != "Bearer" || token["expires_in"] != float64(3600) || f.auth.requestID != fixtureRequestID {
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
	f.dummies.createID = "invalid"
	if response := f.request(http.MethodPost, dummiesPath, createBody, true); response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", response.Code)
	}
	f.dummies.dummy.Name = ""
	if response := f.request(http.MethodGet, dummyPath, "", false); response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", response.Code)
	}
}
func TestCreateWaitsForPublication(t *testing.T) {
	f := newHTTPFixture(t)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	f.dummies.createFn = func(_ context.Context, _, _ string) (string, error) {
		close(started)
		<-release
		return fixtureDummyID, nil
	}
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
