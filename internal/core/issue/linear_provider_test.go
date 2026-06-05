package issue

import (
	"os"
	"testing"
)

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

func TestNewProvider_Linear_MissingAPIKey(t *testing.T) {
	// Ensure env var is not set
	_ = os.Unsetenv("LINEAR_API_KEY")

	_, err := NewProvider(ProviderConfig{
		ProviderType: "linear",
		Config:       map[string]string{"team": "ENG"},
		Deps:         ProviderDeps{HTTP: &mockHTTPClient{}},
	})
	if err == nil {
		t.Error("NewProvider(linear) without API key should fail")
	}
}

func TestNewProvider_Linear_MissingTeam(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "test-key")

	_, err := NewProvider(ProviderConfig{
		ProviderType: "linear",
		Config:       map[string]string{},
		Deps:         ProviderDeps{HTTP: &mockHTTPClient{}},
	})
	if err == nil {
		t.Error("NewProvider(linear) without team should fail")
	}
}

func TestNewProvider_Linear_MissingHTTP(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "test-key")

	_, err := NewProvider(ProviderConfig{
		ProviderType: "linear",
		Config:       map[string]string{"team": "ENG"},
		Deps:         ProviderDeps{},
	})
	if err == nil {
		t.Error("NewProvider(linear) without HTTP client should fail")
	}
}

func TestNewProvider_Linear_Success(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "test-key")

	provider, err := NewProvider(ProviderConfig{
		ProviderType: "linear",
		Config:       map[string]string{"team": "ENG"},
		Deps:         ProviderDeps{HTTP: &mockHTTPClient{}},
	})
	if err != nil {
		t.Fatalf("NewProvider(linear) error = %v", err)
	}
	if _, ok := provider.(*LinearProvider); !ok {
		t.Errorf("NewProvider(linear) returned %T, want *LinearProvider", provider)
	}
}

func TestNewProvider_Linear_APIKeyFromConfig(t *testing.T) {
	// Ensure env var is not set
	_ = os.Unsetenv("LINEAR_API_KEY")

	provider, err := NewProvider(ProviderConfig{
		ProviderType: "linear",
		Config:       map[string]string{"team": "ENG", "api_key": "config-key"},
		Deps:         ProviderDeps{HTTP: &mockHTTPClient{}},
	})
	if err != nil {
		t.Fatalf("NewProvider(linear) with config api_key error = %v", err)
	}
	if _, ok := provider.(*LinearProvider); !ok {
		t.Errorf("NewProvider(linear) returned %T, want *LinearProvider", provider)
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider(ProviderConfig{
		ProviderType: "unknown",
		Config:       map[string]string{},
	})
	if err == nil {
		t.Error("NewProvider(unknown) should fail")
	}
}

func TestRegisteredProviders(t *testing.T) {
	providers := RegisteredProviders()

	hasMarkdown := false
	hasLinear := false
	for _, p := range providers {
		if p == "markdown" {
			hasMarkdown = true
		}
		if p == "linear" {
			hasLinear = true
		}
	}

	if !hasMarkdown {
		t.Error("RegisteredProviders() should include 'markdown'")
	}
	if !hasLinear {
		t.Error("RegisteredProviders() should include 'linear'")
	}
}

func TestMapLinearStatus_UnknownState(t *testing.T) {
	tests := []struct {
		state  string
		expect string
	}{
		{"Unknown", "todo"},
		{"Custom Progress State", "in-progress"},
		{"Complete", "done"},
		{"Cancelled", "done"},
	}

	for _, tt := range tests {
		got := mapLinearStatus(tt.state)
		if got != tt.expect {
			t.Errorf("mapLinearStatus(%q) = %q, want %q", tt.state, got, tt.expect)
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
