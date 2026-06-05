//go:build integration

package issue_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
)

// stubHTTP returns a fixed JSON body for every request.
type stubHTTP struct {
	body     string
	requests int
}

func (s *stubHTTP) Do(req *http.Request) (*http.Response, error) {
	s.requests++
	_, _ = io.Copy(io.Discard, req.Body)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(s.body)),
		Header:     make(http.Header),
	}, nil
}

// writeImportRepo creates a temp repo whose config names linear as the import
// source, plus the local issues directory.
func writeImportRepo(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mp-issue-import-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	configDir := filepath.Join(tmpDir, ".monkeypuzzle")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	cfg := `{"version":"1","project":{"name":"test"},"issues":{"provider":"linear","config":{"directory":"issues","team":"ENG","api_key":"k"}},"pr":{"provider":"github","config":{}}}`
	if err := os.WriteFile(filepath.Join(configDir, "monkeypuzzle.json"), []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "issues"), 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}
	return tmpDir
}

// AC2: mp issue import --from linear --id X writes a local markdown issue
// containing the fetched title + body.
func TestIntegration_Import_Linear_WritesMarkdown(t *testing.T) {
	tmpDir := writeImportRepo(t)

	http := &stubHTTP{body: `{"data":{"issue":{"id":"uuid-1","identifier":"ENG-42","title":"Imported Feature","description":"Body from Linear","url":"https://linear.app/x/issue/ENG-42"}}}`}
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewBufferOutput(),
		nil,
		http,
		nil,
	)
	handler := issue.NewHandler(deps, tmpDir)

	result, err := handler.Import(context.Background(), issue.ImportInput{From: "linear", ID: "ENG-42"})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	if result.Title != "Imported Feature" {
		t.Errorf("Import() title = %q, want Imported Feature", result.Title)
	}

	// The file must exist on disk under issues/ and contain title + body.
	abs := filepath.Join(tmpDir, result.Path)
	content, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("imported issue file not written: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "Imported Feature") {
		t.Errorf("imported file missing title:\n%s", got)
	}
	if !strings.Contains(got, "Body from Linear") {
		t.Errorf("imported file missing body:\n%s", got)
	}
	if !strings.Contains(got, "https://linear.app/x/issue/ENG-42") {
		t.Errorf("imported file missing source URL:\n%s", got)
	}
}

// AC4: a --query matching multiple remote issues with no --id fails loudly and
// names the choices.
func TestIntegration_Import_AmbiguousQuery_Fails(t *testing.T) {
	tmpDir := writeImportRepo(t)

	http := &stubHTTP{body: `{"data":{"issues":{"nodes":[
		{"id":"u1","identifier":"ENG-1","title":"Auth one"},
		{"id":"u2","identifier":"ENG-2","title":"Auth two"}
	]}}}`}
	deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewBufferOutput(), nil, http, nil)
	handler := issue.NewHandler(deps, tmpDir)

	_, err := handler.Import(context.Background(), issue.ImportInput{From: "linear", Query: "auth"})
	if err == nil {
		t.Fatal("Import() with ambiguous query should fail")
	}
	if !strings.Contains(err.Error(), "ENG-1") || !strings.Contains(err.Error(), "ENG-2") {
		t.Errorf("ambiguity error should name the choices, got: %v", err)
	}
}

// AC4: a --query matching exactly one remote issue proceeds.
func TestIntegration_Import_UniqueQuery_Proceeds(t *testing.T) {
	tmpDir := writeImportRepo(t)

	http := &stubHTTP{body: `{"data":{"issues":{"nodes":[
		{"id":"u1","identifier":"ENG-9","title":"Only match","description":"b"}
	]}}}`}
	deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewBufferOutput(), nil, http, nil)
	handler := issue.NewHandler(deps, tmpDir)

	result, err := handler.Import(context.Background(), issue.ImportInput{From: "linear", Query: "only"})
	if err != nil {
		t.Fatalf("Import() unique query error = %v", err)
	}
	if result.Title != "Only match" {
		t.Errorf("Import() title = %q, want Only match", result.Title)
	}
}

// AC5: backward compat — a config with issue_provider: linear keeps the local
// markdown store working (create + list operate on issues/*.md), and surfaces
// linear as an import source.
func TestIntegration_BackwardCompat_LinearConfig_LocalStoreIsMarkdown(t *testing.T) {
	tmpDir := writeImportRepo(t)

	deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewBufferOutput(), nil, nil, nil)
	handler := issue.NewHandler(deps, tmpDir)

	// create writes local markdown despite issue_provider: linear
	created, err := handler.Run(issue.Input{Title: "Local Issue"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.HasSuffix(created.Path, ".md") {
		t.Errorf("created issue path %q is not a markdown file", created.Path)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, created.Path)); err != nil {
		t.Fatalf("local markdown issue not written: %v", err)
	}

	// list returns the local issue
	items, err := handler.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(items) != 1 || items[0].Provider != "markdown" {
		t.Errorf("ListIssues() = %+v, want one markdown issue", items)
	}

	// linear is offered as an import source
	sources, err := handler.ConfiguredImportSources()
	if err != nil {
		t.Fatalf("ConfiguredImportSources() error = %v", err)
	}
	if len(sources) != 1 || sources[0].Name != "linear" {
		t.Errorf("ConfiguredImportSources() = %+v, want [linear]", sources)
	}
}
