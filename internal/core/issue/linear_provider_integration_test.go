package issue

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mockHTTPClient implements core.HTTPClient for testing the Linear importer.
type mockHTTPClient struct {
	requests  []*http.Request
	responses map[string]string // operation -> response JSON
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Clone request body so it can be read again in tests
	body, _ := io.ReadAll(req.Body)
	bodyStr := string(body)
	req.Body = io.NopCloser(bytes.NewBufferString(bodyStr))
	m.requests = append(m.requests, req)

	var responseJSON string
	switch {
	case strings.Contains(bodyStr, "issues"):
		responseJSON = m.responses["issues"]
	case strings.Contains(bodyStr, "issue"):
		responseJSON = m.responses["issue"]
	default:
		responseJSON = `{"data":{}}`
	}

	if responseJSON == "" {
		responseJSON = `{"data":{}}`
	}

	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(responseJSON)),
		Header:     make(http.Header),
	}, nil
}

func TestLinearImporter_Fetch(t *testing.T) {
	mockHTTP := &mockHTTPClient{
		responses: map[string]string{
			"issue": `{"data":{"issue":{"id":"abc123","identifier":"ENG-7","title":"Test Issue","description":"Test description","url":"https://linear.app/x/issue/ENG-7"}}}`,
		},
	}

	imp := NewLinearImporter(mockHTTP, "test-api-key", "ENG")

	got, err := imp.Fetch(context.Background(), "ENG-7")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.ID != "ENG-7" {
		t.Errorf("Fetch() ID = %v, want ENG-7", got.ID)
	}
	if got.Title != "Test Issue" {
		t.Errorf("Fetch() Title = %v, want Test Issue", got.Title)
	}
	if got.Body != "Test description" {
		t.Errorf("Fetch() Body = %v, want Test description", got.Body)
	}
	if got.URL != "https://linear.app/x/issue/ENG-7" {
		t.Errorf("Fetch() URL = %v", got.URL)
	}

	// Verify API key was sent in requests
	for _, req := range mockHTTP.requests {
		if auth := req.Header.Get("Authorization"); auth != "test-api-key" {
			t.Errorf("Request missing/wrong Authorization header: %v", auth)
		}
	}
}

func TestLinearImporter_Fetch_NotFound(t *testing.T) {
	mockHTTP := &mockHTTPClient{
		responses: map[string]string{"issue": `{"data":{"issue":null}}`},
	}
	imp := NewLinearImporter(mockHTTP, "k", "ENG")
	if _, err := imp.Fetch(context.Background(), "ENG-999"); err == nil {
		t.Error("Fetch() of missing issue should error")
	}
}

func TestLinearImporter_Search_UsesServerSideFilter(t *testing.T) {
	mockHTTP := &mockHTTPClient{
		responses: map[string]string{
			"issues": `{"data":{"issues":{"nodes":[
				{"id":"match1","identifier":"ENG-123","title":"Auth feature","description":"Add authentication"}
			]}}}`,
		},
	}

	imp := NewLinearImporter(mockHTTP, "key", "TEAM")

	issues, err := imp.Search(context.Background(), "auth", 0)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(mockHTTP.requests) == 0 {
		t.Fatal("No requests made")
	}
	lastReq := mockHTTP.requests[len(mockHTTP.requests)-1]
	body, _ := io.ReadAll(lastReq.Body)
	if !strings.Contains(string(body), "containsIgnoreCase") {
		t.Errorf("Expected containsIgnoreCase filter in query, got: %s", string(body))
	}

	if len(issues) != 1 {
		t.Fatalf("Search() returned %d issues, want 1", len(issues))
	}
	if issues[0].ID != "ENG-123" {
		t.Errorf("Search() returned wrong issue ID: %v", issues[0].ID)
	}
}

func TestLinearImporter_Search_EmptyQuery_NoFilter(t *testing.T) {
	mockHTTP := &mockHTTPClient{
		responses: map[string]string{
			"issues": `{"data":{"issues":{"nodes":[
				{"id":"1","identifier":"ENG-1","title":"Issue 1"},
				{"id":"2","identifier":"ENG-2","title":"Issue 2"}
			]}}}`,
		},
	}

	imp := NewLinearImporter(mockHTTP, "key", "TEAM")

	issues, err := imp.Search(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("Search() returned %d issues, want 2", len(issues))
	}

	lastReq := mockHTTP.requests[len(mockHTTP.requests)-1]
	body, _ := io.ReadAll(lastReq.Body)
	if strings.Contains(string(body), "containsIgnoreCase") {
		t.Errorf("Empty query should not use title filter, got: %s", string(body))
	}
}
