package issue

import (
	"os"
	"testing"
)

func TestNewImporter_Linear_MissingAPIKey(t *testing.T) {
	_ = os.Unsetenv("LINEAR_API_KEY")

	_, err := NewImporter(ImporterConfig{
		Source: "linear",
		Config: map[string]string{"team": "ENG"},
		Deps:   ImporterDeps{HTTP: &mockHTTPClient{}},
	})
	if err == nil {
		t.Error("NewImporter(linear) without API key should fail")
	}
}

func TestNewImporter_Linear_MissingTeam(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "test-key")

	_, err := NewImporter(ImporterConfig{
		Source: "linear",
		Config: map[string]string{},
		Deps:   ImporterDeps{HTTP: &mockHTTPClient{}},
	})
	if err == nil {
		t.Error("NewImporter(linear) without team should fail")
	}
}

func TestNewImporter_Linear_MissingHTTP(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "test-key")

	_, err := NewImporter(ImporterConfig{
		Source: "linear",
		Config: map[string]string{"team": "ENG"},
		Deps:   ImporterDeps{},
	})
	if err == nil {
		t.Error("NewImporter(linear) without HTTP client should fail")
	}
}

func TestNewImporter_Linear_Success(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "test-key")

	imp, err := NewImporter(ImporterConfig{
		Source: "linear",
		Config: map[string]string{"team": "ENG"},
		Deps:   ImporterDeps{HTTP: &mockHTTPClient{}},
	})
	if err != nil {
		t.Fatalf("NewImporter(linear) error = %v", err)
	}
	if _, ok := imp.(*LinearImporter); !ok {
		t.Errorf("NewImporter(linear) returned %T, want *LinearImporter", imp)
	}
}

func TestNewImporter_Linear_APIKeyFromConfig(t *testing.T) {
	_ = os.Unsetenv("LINEAR_API_KEY")

	imp, err := NewImporter(ImporterConfig{
		Source: "linear",
		Config: map[string]string{"team": "ENG", "api_key": "config-key"},
		Deps:   ImporterDeps{HTTP: &mockHTTPClient{}},
	})
	if err != nil {
		t.Fatalf("NewImporter(linear) with config api_key error = %v", err)
	}
	if _, ok := imp.(*LinearImporter); !ok {
		t.Errorf("NewImporter(linear) returned %T, want *LinearImporter", imp)
	}
}

func TestNewImporter_Unknown(t *testing.T) {
	_, err := NewImporter(ImporterConfig{Source: "unknown"})
	if err == nil {
		t.Error("NewImporter(unknown) should fail")
	}
}

func TestRegisteredImporters(t *testing.T) {
	importers := RegisteredImporters()

	hasLinear := false
	hasPlane := false
	for _, p := range importers {
		if p == "linear" {
			hasLinear = true
		}
		if p == "plane" {
			hasPlane = true
		}
	}
	if !hasLinear {
		t.Error("RegisteredImporters() should include 'linear'")
	}
	if !hasPlane {
		t.Error("RegisteredImporters() should include 'plane'")
	}
}

// TestNewProvider_Markdown verifies the local store is still markdown-only.
func TestNewProvider_Markdown(t *testing.T) {
	provider, err := NewProvider(ProviderConfig{
		ProviderType: "markdown",
		Config:       map[string]string{"directory": "/tmp/issues"},
		Deps:         ProviderDeps{FS: &mockFS{}},
	})
	if err != nil {
		t.Fatalf("NewProvider(markdown) error = %v", err)
	}
	if _, ok := provider.(*MarkdownProvider); !ok {
		t.Errorf("NewProvider(markdown) returned %T, want *MarkdownProvider", provider)
	}
}

// TestNewProvider_TrackerNotALocalStore confirms trackers are no longer local
// store providers.
func TestNewProvider_TrackerNotALocalStore(t *testing.T) {
	for _, name := range []string{"linear", "plane"} {
		if _, err := NewProvider(ProviderConfig{ProviderType: name}); err == nil {
			t.Errorf("NewProvider(%q) should fail; trackers are import sources, not local stores", name)
		}
	}
}

// mockFS implements core.FS for testing
type mockFS struct{}

func (m *mockFS) MkdirAll(path string, perm os.FileMode) error               { return nil }
func (m *mockFS) WriteFile(name string, data []byte, perm os.FileMode) error { return nil }
func (m *mockFS) ReadFile(name string) ([]byte, error)                       { return nil, nil }
func (m *mockFS) Stat(name string) (os.FileInfo, error)                      { return nil, os.ErrNotExist }
func (m *mockFS) Remove(name string) error                                   { return nil }
func (m *mockFS) Symlink(oldname, newname string) error                      { return nil }
func (m *mockFS) ReadDir(name string) ([]os.DirEntry, error)                 { return nil, nil }
