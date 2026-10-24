package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSwaggerContract(t *testing.T) {
	spec, err := GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(spec.Paths) != 4 || spec.Paths["/api/v1/companies"] != nil {
		t.Fatal("unexpected paths")
	}
	security := spec.Components.SecuritySchemes["BearerAuth"].Value
	if security.Type != "http" || security.Scheme != "bearer" {
		t.Fatal("expected HTTP bearer security")
	}
	create := spec.Paths["/api/v1/dummies"].Post
	patch := spec.Paths["/api/v1/dummies/{id}"].Patch
	remove := spec.Paths["/api/v1/dummies/{id}"].Delete
	for _, operation := range []string{"post", "patch", "delete"} {
		op := create
		if operation == "patch" {
			op = patch
		}
		if operation == "delete" {
			op = remove
		}
		if op.Responses["202"] == nil || op.Responses["401"] == nil || op.Responses["404"] != nil ||
			op.Security == nil {
			t.Fatalf("invalid asynchronous write contract: %s", operation)
		}
	}
	if create.Responses["409"] == nil || len(create.Responses["409"].Value.Content) != 0 ||
		patch.Responses["409"] != nil ||
		remove.Responses["409"] != nil {
		t.Fatal("only create must document an empty duplicate-name conflict response")
	}
	if len(patch.Responses["202"].Value.Content) != 0 || len(remove.Responses["202"].Value.Content) != 0 {
		t.Fatal("patch/delete acceptance must have empty bodies")
	}
	for _, tc := range []struct {
		name  string
		patch bool
		body  string
		valid bool
	}{
		{"create", false, `{"name":"Example"}`, true},
		{"missing name", false, `{}`, false},
		{"blank name", false, `{"name":"  "}`, false},
		{"null name", false, `{"name":null}`, false},
		{"null note", false, `{"name":"Example","note":null}`, false},
		{"empty patch", true, `{}`, false},
		{"unrecognized patch", true, `{"other":"value"}`, false},
		{"clear note", true, `{"note":""}`, true},
		{"patch name", true, `{"name":"Example"}`, true},
		{"patch null", true, `{"note":null}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op := create
			if tc.patch {
				op = patch
			}
			var value any
			if err := json.Unmarshal([]byte(tc.body), &value); err != nil {
				t.Fatal(err)
			}
			err := op.RequestBody.Value.Content["application/json"].Schema.Value.VisitJSON(value)
			if (err == nil) != tc.valid {
				t.Fatalf("validation error = %v, valid = %v", err, tc.valid)
			}
		})
	}
}

func TestSwaggerRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := RegisterSwagger(router.Group("/api/v1")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/documentation/", "/api/v1/docs.json"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d", response.Code)
			}
			if path == "/api/v1/docs.json" {
				var document struct {
					Paths map[string]any `json:"paths"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
					t.Fatal(err)
				}
				if document.Paths["/api/v1/dummies"] == nil {
					t.Fatal("served spec is missing Dummy")
				}
			}
		})
	}
}
