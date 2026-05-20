package issue

import (
	"bytes"
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

func newPlaneTestProvider(routes map[string]string) (*PlaneProvider, *planeMockTransport) {
	mt := &planeMockTransport{routes: routes}
	return NewPlaneProvider(mt, "https://api.plane.so", "test-key", planeWS, planeProj), mt
}

// statesResponse is a minimal Plane states list with the five default groups.
const statesResponse = `{"results":[
	{"id":"s-backlog","group":"backlog"},
	{"id":"s-unstarted","group":"unstarted"},
	{"id":"s-started","group":"started"},
	{"id":"s-completed","group":"completed"},
	{"id":"s-cancelled","group":"cancelled"}
],"next_page_results":false,"next_cursor":""}`

const projectResponse = `{"id":"proj-uuid","identifier":"PROJ","name":"My Project"}`

func TestNewProvider_Plane_MissingAPIKey(t *testing.T) {
	os.Unsetenv("PLANE_API_KEY")
	_, err := NewProvider(ProviderConfig{
		ProviderType: "plane",
		Config:       map[string]string{"workspace": "acme", "project": "p"},
		Deps:         ProviderDeps{HTTP: &planeMockTransport{}},
	})
	if err == nil {
		t.Error("NewProvider(plane) without API key should fail")
	}
}

func TestNewProvider_Plane_MissingWorkspace(t *testing.T) {
	os.Setenv("PLANE_API_KEY", "k")
	defer os.Unsetenv("PLANE_API_KEY")
	_, err := NewProvider(ProviderConfig{
		ProviderType: "plane",
		Config:       map[string]string{"project": "p"},
		Deps:         ProviderDeps{HTTP: &planeMockTransport{}},
	})
	if err == nil {
		t.Error("NewProvider(plane) without workspace should fail")
	}
}

func TestNewProvider_Plane_MissingProject(t *testing.T) {
	os.Setenv("PLANE_API_KEY", "k")
	defer os.Unsetenv("PLANE_API_KEY")
	_, err := NewProvider(ProviderConfig{
		ProviderType: "plane",
		Config:       map[string]string{"workspace": "acme"},
		Deps:         ProviderDeps{HTTP: &planeMockTransport{}},
	})
	if err == nil {
		t.Error("NewProvider(plane) without project should fail")
	}
}

func TestNewProvider_Plane_Success(t *testing.T) {
	os.Setenv("PLANE_API_KEY", "k")
	defer os.Unsetenv("PLANE_API_KEY")
	provider, err := NewProvider(ProviderConfig{
		ProviderType: "plane",
		Config:       map[string]string{"workspace": "acme", "project": "p"},
		Deps:         ProviderDeps{HTTP: &planeMockTransport{}},
	})
	if err != nil {
		t.Fatalf("NewProvider(plane) error = %v", err)
	}
	if _, ok := provider.(*PlaneProvider); !ok {
		t.Errorf("NewProvider(plane) returned %T, want *PlaneProvider", provider)
	}
}

func TestNewProvider_Plane_DefaultBaseURL(t *testing.T) {
	os.Unsetenv("PLANE_API_KEY")
	provider, err := NewProvider(ProviderConfig{
		ProviderType: "plane",
		Config:       map[string]string{"workspace": "acme", "project": "p", "api_key": "k"},
		Deps:         ProviderDeps{HTTP: &planeMockTransport{}},
	})
	if err != nil {
		t.Fatalf("NewProvider(plane) error = %v", err)
	}
	pp := provider.(*PlaneProvider)
	if pp.baseURL != DefaultPlaneBaseURL {
		t.Errorf("baseURL = %q, want %q", pp.baseURL, DefaultPlaneBaseURL)
	}
}

func TestPlaneProvider_HappyPath(t *testing.T) {
	routes := map[string]string{
		"GET " + planeBasePath(""):             projectResponse,
		"GET " + planeBasePath("states/"):      statesResponse,
		"POST " + planeBasePath("issues/"):     `{"id":"i1","sequence_id":7,"name":"New issue","description_stripped":"body","state":"s-backlog"}`,
		"GET " + planeBasePath("issues/"):      `{"results":[{"id":"i1","sequence_id":7,"name":"New issue","description_stripped":"body","state":"s-backlog"},{"id":"i2","sequence_id":8,"name":"Doing","description_stripped":"","state":"s-started"},{"id":"i3","sequence_id":9,"name":"Done one","description_stripped":"","state":"s-completed"}],"next_page_results":false,"next_cursor":""}`,
		"GET " + planeBasePath("issues/i1/"):   `{"id":"i1","sequence_id":7,"name":"New issue","description_stripped":"body","state":"s-backlog"}`,
		"PATCH " + planeBasePath("issues/i1/"): `{"id":"i1","sequence_id":7,"name":"New issue","state":"s-started"}`,
	}
	provider, mt := newPlaneTestProvider(routes)

	// Create
	created, err := provider.Create(CreateInput{Title: "New issue", Description: "body"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != "i1" || created.Number != "PROJ-7" || created.Title != "New issue" {
		t.Errorf("Create() = %+v", created)
	}
	if created.Status != "todo" {
		t.Errorf("Create() Status = %q, want todo", created.Status)
	}

	// List
	issues, err := provider.List(nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("List() returned %d issues, want 3", len(issues))
	}

	// Get
	got, err := provider.Get("i1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Title != "New issue" || got.Description != "body" {
		t.Errorf("Get() = %+v", got)
	}

	// UpdateStatus -> PATCH with started state uuid
	if err := provider.UpdateStatus("i1", "in-progress"); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	var patchReq *http.Request
	for _, r := range mt.requests {
		if r.Method == "PATCH" {
			patchReq = r
		}
	}
	if patchReq == nil {
		t.Fatal("no PATCH request issued")
	}
	body, _ := io.ReadAll(patchReq.Body)
	if !strings.Contains(string(body), "s-started") {
		t.Errorf("PATCH body = %s, want state s-started", string(body))
	}

	// All requests carry the API key header
	for _, r := range mt.requests {
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("request %s %s missing X-API-Key header", r.Method, r.URL.Path)
		}
	}
}

func TestPlaneProvider_ListStatusFilter(t *testing.T) {
	routes := map[string]string{
		"GET " + planeBasePath(""):        projectResponse,
		"GET " + planeBasePath("states/"): statesResponse,
		"GET " + planeBasePath("issues/"): `{"results":[{"id":"i1","sequence_id":1,"name":"a","state":"s-backlog"},{"id":"i2","sequence_id":2,"name":"b","state":"s-started"},{"id":"i3","sequence_id":3,"name":"c","state":"s-completed"}],"next_page_results":false,"next_cursor":""}`,
	}
	provider, _ := newPlaneTestProvider(routes)

	issues, err := provider.List([]string{"in-progress"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "i2" {
		t.Errorf("List(in-progress) = %+v", issues)
	}
}

func TestPlaneProvider_SearchQueryFiltersByTitle(t *testing.T) {
	routes := map[string]string{
		"GET " + planeBasePath(""):        projectResponse,
		"GET " + planeBasePath("states/"): statesResponse,
		"GET " + planeBasePath("issues/"): `{"results":[{"id":"i1","sequence_id":1,"name":"Add auth flow","state":"s-backlog"},{"id":"i2","sequence_id":2,"name":"Fix typo","state":"s-backlog"}],"next_page_results":false,"next_cursor":""}`,
	}
	provider, _ := newPlaneTestProvider(routes)

	issues, err := provider.SearchIssues(SearchInput{Query: "AUTH"})
	if err != nil {
		t.Fatalf("SearchIssues() error = %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "i1" {
		t.Errorf("SearchIssues(auth) = %+v", issues)
	}
}

func TestPlaneProvider_Pagination(t *testing.T) {
	// First page points to a cursor; second page ends the walk.
	mt := &planeMockTransport{routes: map[string]string{
		"GET " + planeBasePath(""):        projectResponse,
		"GET " + planeBasePath("states/"): statesResponse,
	}}
	// Custom Do: branch on cursor query param.
	provider := NewPlaneProvider(&paginatingTransport{inner: mt}, "https://api.plane.so", "k", planeWS, planeProj)
	issues, err := provider.List(nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("List() across pages returned %d, want 2", len(issues))
	}
}

// paginatingTransport serves a two-page issues list, delegating everything else to inner.
type paginatingTransport struct{ inner *planeMockTransport }

func (p *paginatingTransport) Do(req *http.Request) (*http.Response, error) {
	if req.URL.Path == planeBasePath("issues/") {
		var body string
		if req.URL.Query().Get("cursor") == "page2" {
			body = `{"results":[{"id":"i2","sequence_id":2,"name":"two","state":"s-backlog"}],"next_page_results":false,"next_cursor":""}`
		} else {
			body = `{"results":[{"id":"i1","sequence_id":1,"name":"one","state":"s-backlog"}],"next_page_results":true,"next_cursor":"page2"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}
	return p.inner.Do(req)
}

func TestMpStatusToPlaneGroup(t *testing.T) {
	cases := map[string]string{
		"todo":        "backlog",
		"in-progress": "started",
		"done":        "completed",
		"weird":       "backlog",
	}
	for in, want := range cases {
		if got := mpStatusToPlaneGroup(in); got != want {
			t.Errorf("mpStatusToPlaneGroup(%q) = %q, want %q", in, got, want)
		}
	}
}
