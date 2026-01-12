package issue

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mockHTTPClient implements core.HTTPClient for testing
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
	case strings.Contains(bodyStr, "issueCreate"):
		responseJSON = m.responses["createIssue"]
	case strings.Contains(bodyStr, "issueUpdate"):
		responseJSON = m.responses["updateIssue"]
	case strings.Contains(bodyStr, "searchIssues"):
		responseJSON = m.responses["searchIssues"]
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

func TestLinearProvider_HappyPath(t *testing.T) {
	mockHTTP := &mockHTTPClient{
		responses: map[string]string{
			"createIssue": `{"data":{"issueCreate":{"issue":{"id":"abc123","title":"Test Issue","description":"Test description","state":{"name":"Backlog"}}}}}`,
			"issues":      `{"data":{"issues":{"nodes":[{"id":"abc123","title":"Test Issue","description":"Test description","state":{"name":"Backlog"}}]}}}`,
			"issue":       `{"data":{"issue":{"id":"abc123","title":"Test Issue","description":"Test description","state":{"name":"Backlog"}}}}`,
			"updateIssue": `{"data":{"issueUpdate":{"issue":{"id":"abc123","state":{"name":"In Progress"}}}}}`,
		},
	}

	provider := NewLinearProvider(mockHTTP, "test-api-key", "ENG")

	// Test Create
	issue, err := provider.Create(CreateInput{Title: "Test Issue", Description: "Test description"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if issue.ID != "abc123" {
		t.Errorf("Create() ID = %v, want abc123", issue.ID)
	}
	if issue.Title != "Test Issue" {
		t.Errorf("Create() Title = %v, want Test Issue", issue.Title)
	}
	if issue.Status != "todo" {
		t.Errorf("Create() Status = %v, want todo (mapped from Backlog)", issue.Status)
	}

	// Test List
	issues, err := provider.List(nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("List() returned %d issues, want 1", len(issues))
	}
	if len(issues) > 0 && issues[0].Status != "todo" {
		t.Errorf("List() issue status = %v, want todo", issues[0].Status)
	}

	// Test Get
	issue, err = provider.Get("abc123")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if issue.Title != "Test Issue" {
		t.Errorf("Get() Title = %v, want Test Issue", issue.Title)
	}

	// Test UpdateStatus
	err = provider.UpdateStatus("abc123", "in-progress")
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	// Verify API key was sent in requests
	for _, req := range mockHTTP.requests {
		auth := req.Header.Get("Authorization")
		if auth != "test-api-key" {
			t.Errorf("Request missing/wrong Authorization header: %v", auth)
		}
	}
}

func TestLinearProvider_StatusMapping(t *testing.T) {
	tests := []struct {
		linearState string
		wantStatus  string
	}{
		{"Backlog", "todo"},
		{"Todo", "todo"},
		{"In Progress", "in-progress"},
		{"In Review", "in-progress"},
		{"Done", "done"},
		{"Canceled", "done"},
	}

	for _, tt := range tests {
		t.Run(tt.linearState, func(t *testing.T) {
			mockHTTP := &mockHTTPClient{
				responses: map[string]string{
					"issue": `{"data":{"issue":{"id":"test","title":"Test","state":{"name":"` + tt.linearState + `"}}}}`,
				},
			}

			provider := NewLinearProvider(mockHTTP, "key", "TEAM")
			issue, err := provider.Get("test")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if issue.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v (from Linear state %v)", issue.Status, tt.wantStatus, tt.linearState)
			}
		})
	}
}

func TestLinearProvider_ListWithStatusFilter(t *testing.T) {
	mockHTTP := &mockHTTPClient{
		responses: map[string]string{
			"issues": `{"data":{"issues":{"nodes":[
				{"id":"1","title":"Todo","state":{"name":"Backlog"}},
				{"id":"2","title":"InProgress","state":{"name":"In Progress"}},
				{"id":"3","title":"Done","state":{"name":"Done"}}
			]}}}`,
		},
	}

	provider := NewLinearProvider(mockHTTP, "key", "TEAM")

	// Filter for todo only
	issues, err := provider.List([]string{"todo"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("List(todo) returned %d issues, want 1", len(issues))
	}
	if len(issues) > 0 && issues[0].ID != "1" {
		t.Errorf("List(todo) returned wrong issue: %v", issues[0].ID)
	}

	// Filter for in-progress only
	issues, err = provider.List([]string{"in-progress"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("List(in-progress) returned %d issues, want 1", len(issues))
	}
}

func TestLinearProvider_SearchIssues_UsesServerSideFilter(t *testing.T) {
	mockHTTP := &mockHTTPClient{
		responses: map[string]string{
			"issues": `{"data":{"issues":{"nodes":[
				{"id":"match1","identifier":"ENG-123","title":"Auth feature","description":"Add authentication","state":{"name":"Todo"}}
			]}}}`,
		},
	}

	provider := NewLinearProvider(mockHTTP, "key", "TEAM")

	// Search with query should use issues with containsIgnoreCase filter
	issues, err := provider.SearchIssues(SearchInput{Query: "auth"})
	if err != nil {
		t.Fatalf("SearchIssues() error = %v", err)
	}

	// Verify issues query was called with filter
	if len(mockHTTP.requests) == 0 {
		t.Fatal("No requests made")
	}

	lastReq := mockHTTP.requests[len(mockHTTP.requests)-1]
	body, _ := io.ReadAll(lastReq.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "containsIgnoreCase") {
		t.Errorf("Expected containsIgnoreCase filter in query, got: %s", bodyStr)
	}

	// Should return filtered results
	if len(issues) != 1 {
		t.Errorf("SearchIssues() returned %d issues, want 1", len(issues))
	}
	if len(issues) > 0 && issues[0].ID != "match1" {
		t.Errorf("SearchIssues() returned wrong issue ID: %v", issues[0].ID)
	}
}

func TestLinearProvider_SearchIssues_EmptyQuery_NoFilter(t *testing.T) {
	mockHTTP := &mockHTTPClient{
		responses: map[string]string{
			"issues": `{"data":{"issues":{"nodes":[
				{"id":"1","identifier":"ENG-1","title":"Issue 1","state":{"name":"Backlog"}},
				{"id":"2","identifier":"ENG-2","title":"Issue 2","state":{"name":"Todo"}}
			]}}}`,
		},
	}

	provider := NewLinearProvider(mockHTTP, "key", "TEAM")

	// Empty query should use regular issues list without title filter
	issues, err := provider.SearchIssues(SearchInput{})
	if err != nil {
		t.Fatalf("SearchIssues() error = %v", err)
	}

	if len(issues) != 2 {
		t.Errorf("SearchIssues() returned %d issues, want 2", len(issues))
	}

	// Verify issues query was used without containsIgnoreCase filter
	lastReq := mockHTTP.requests[len(mockHTTP.requests)-1]
	body, _ := io.ReadAll(lastReq.Body)
	bodyStr := string(body)

	if strings.Contains(bodyStr, "containsIgnoreCase") {
		t.Errorf("Empty query should not use title filter, got: %s", bodyStr)
	}
}
