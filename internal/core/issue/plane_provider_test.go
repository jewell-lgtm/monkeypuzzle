package issue

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// planeMockTransport routes requests by method + URL path to canned JSON responses.
type planeMockTransport struct {
	requests []*http.Request
	// route key: "<METHOD> <path>" (path only, no query); value: response JSON body
	routes map[string]string
}

func (m *planeMockTransport) Do(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	m.requests = append(m.requests, req)

	key := req.Method + " " + req.URL.Path
	body, ok := m.routes[key]
	if !ok {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(strings.NewReader(`{"error":"not found: ` + key + `"}`)),
			Header:     make(http.Header),
		}, nil
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

const (
	planeWS   = "acme"
	planeProj = "proj-uuid"
)

func planeBasePath(suffix string) string {
	return "/api/v1/workspaces/" + planeWS + "/projects/" + planeProj + "/" + suffix
}

func newPlaneTestImporter(routes map[string]string) (*PlaneImporter, *planeMockTransport) {
	mt := &planeMockTransport{routes: routes}
	return NewPlaneImporter(mt, "https://api.plane.so", "test-key", planeWS, planeProj), mt
}

const projectResponse = `{"id":"proj-uuid","identifier":"PROJ","name":"My Project"}`

func TestNewImporter_Plane_MissingAPIKey(t *testing.T) {
	_ = os.Unsetenv("PLANE_API_KEY")
	_, err := NewImporter(ImporterConfig{
		Source: "plane",
		Config: map[string]string{"workspace": "acme", "project": "p"},
		Deps:   ImporterDeps{HTTP: &planeMockTransport{}},
	})
	if err == nil {
		t.Error("NewImporter(plane) without API key should fail")
	}
}

func TestNewImporter_Plane_MissingWorkspace(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	_, err := NewImporter(ImporterConfig{
		Source: "plane",
		Config: map[string]string{"project": "p"},
		Deps:   ImporterDeps{HTTP: &planeMockTransport{}},
	})
	if err == nil {
		t.Error("NewImporter(plane) without workspace should fail")
	}
}

func TestNewImporter_Plane_MissingProject(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	_, err := NewImporter(ImporterConfig{
		Source: "plane",
		Config: map[string]string{"workspace": "acme"},
		Deps:   ImporterDeps{HTTP: &planeMockTransport{}},
	})
	if err == nil {
		t.Error("NewImporter(plane) without project should fail")
	}
}

func TestNewImporter_Plane_Success(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	imp, err := NewImporter(ImporterConfig{
		Source: "plane",
		Config: map[string]string{"workspace": "acme", "project": "p"},
		Deps:   ImporterDeps{HTTP: &planeMockTransport{}},
	})
	if err != nil {
		t.Fatalf("NewImporter(plane) error = %v", err)
	}
	if _, ok := imp.(*PlaneImporter); !ok {
		t.Errorf("NewImporter(plane) returned %T, want *PlaneImporter", imp)
	}
}

func TestNewImporter_Plane_DefaultBaseURL(t *testing.T) {
	_ = os.Unsetenv("PLANE_API_KEY")
	imp, err := NewImporter(ImporterConfig{
		Source: "plane",
		Config: map[string]string{"workspace": "acme", "project": "p", "api_key": "k"},
		Deps:   ImporterDeps{HTTP: &planeMockTransport{}},
	})
	if err != nil {
		t.Fatalf("NewImporter(plane) error = %v", err)
	}
	pp := imp.(*PlaneImporter)
	if pp.baseURL != DefaultPlaneBaseURL {
		t.Errorf("baseURL = %q, want %q", pp.baseURL, DefaultPlaneBaseURL)
	}
}

func TestPlaneImporter_Fetch(t *testing.T) {
	routes := map[string]string{
		"GET " + planeBasePath(""):           projectResponse,
		"GET " + planeBasePath("issues/i1/"): `{"id":"i1","sequence_id":7,"name":"New issue","description_stripped":"body"}`,
	}
	imp, mt := newPlaneTestImporter(routes)

	got, err := imp.Fetch(context.Background(), "i1")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.ID != "PROJ-7" || got.Title != "New issue" || got.Body != "body" {
		t.Errorf("Fetch() = %+v", got)
	}

	for _, r := range mt.requests {
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("request %s %s missing X-API-Key header", r.Method, r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("importer must be read-only, saw %s %s", r.Method, r.URL.Path)
		}
	}
}

func TestPlaneImporter_SearchQueryFiltersByTitle(t *testing.T) {
	routes := map[string]string{
		"GET " + planeBasePath(""):        projectResponse,
		"GET " + planeBasePath("issues/"): `{"results":[{"id":"i1","sequence_id":1,"name":"Add auth flow"},{"id":"i2","sequence_id":2,"name":"Fix typo"}],"next_page_results":false,"next_cursor":""}`,
	}
	imp, _ := newPlaneTestImporter(routes)

	issues, err := imp.Search(context.Background(), "AUTH", 0)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "PROJ-1" {
		t.Errorf("Search(auth) = %+v", issues)
	}
}

func TestPlaneImporter_Pagination(t *testing.T) {
	mt := &planeMockTransport{routes: map[string]string{
		"GET " + planeBasePath(""): projectResponse,
	}}
	imp := NewPlaneImporter(&paginatingTransport{inner: mt}, "https://api.plane.so", "k", planeWS, planeProj)
	issues, err := imp.Search(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("Search() across pages returned %d, want 2", len(issues))
	}
}

// paginatingTransport serves a two-page issues list, delegating everything else to inner.
type paginatingTransport struct{ inner *planeMockTransport }

func (p *paginatingTransport) Do(req *http.Request) (*http.Response, error) {
	if req.URL.Path == planeBasePath("issues/") {
		var body string
		if req.URL.Query().Get("cursor") == "page2" {
			body = `{"results":[{"id":"i2","sequence_id":2,"name":"two"}],"next_page_results":false,"next_cursor":""}`
		} else {
			body = `{"results":[{"id":"i1","sequence_id":1,"name":"one"}],"next_page_results":true,"next_cursor":"page2"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}
	return p.inner.Do(req)
}
