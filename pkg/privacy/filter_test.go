package privacy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilterLiteral(t *testing.T) {
	f := NewFilter([]string{"password", "API_KEY"})
	blocked, pattern := f.IsBlocked("my password is secret")
	if !blocked {
		t.Errorf("expected 'password' to be blocked")
	}
	if pattern != "password" {
		t.Errorf("expected pattern 'password', got '%s'", pattern)
	}
}

func TestFilterCaseInsensitive(t *testing.T) {
	f := NewFilter([]string{"secret"})
	blocked, _ := f.IsBlocked("My SECRET token")
	if !blocked {
		t.Errorf("expected case-insensitive match")
	}
}

func TestFilterNotBlocked(t *testing.T) {
	f := NewFilter([]string{"password"})
	blocked, _ := f.IsBlocked("This is a normal decision about authentication")
	if blocked {
		t.Errorf("expected not blocked")
	}
}

func TestFilterMultiplePatterns(t *testing.T) {
	f := NewFilter([]string{"*.pem", "secret", "API_KEY*"})
	tests := []struct {
		content string
		blocked bool
	}{
		{"-----BEGIN CERTIFICATE-----\nkey.pem", true},
		{"the secret key is 123", true},
		{"API_KEY=abc123", true},
		{"normal decision about tables", false},
	}
	for _, tt := range tests {
		blocked, _ := f.IsBlocked(tt.content)
		if blocked != tt.blocked {
			t.Errorf("IsBlocked(%q) = %v, want %v", tt.content, blocked, tt.blocked)
		}
	}
}

func TestFilterFromFile(t *testing.T) {
	dir, _ := os.MkdirTemp("", "mcpignore-*")
	defer os.RemoveAll(dir)

	content := "# Sensitive patterns\npassword\nAPI_KEY\n*.pem\n"
	path := filepath.Join(dir, ".mcpignore")
	os.WriteFile(path, []byte(content), 0644)

	f, err := NewFilterFromFile(path)
	if err != nil {
		t.Fatalf("failed to load filter: %v", err)
	}

	tests := []struct {
		content string
		blocked bool
	}{
		{"password=123", true},
		{"API_KEY=abc", true},
		{"certificate content ending with key.pem", true},
		{"normal decision", false},
	}
	for _, tt := range tests {
		blocked, _ := f.IsBlocked(tt.content)
		if blocked != tt.blocked {
			t.Errorf("IsBlocked(%q) = %v, want %v", tt.content, blocked, tt.blocked)
		}
	}
}

func TestFilterCommentsAndBlanks(t *testing.T) {
	content := "# comment\n\npassword\n\n# another comment\n"
	dir, _ := os.MkdirTemp("", "mcpignore-*")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, ".mcpignore")
	os.WriteFile(path, []byte(content), 0644)

	f, err := NewFilterFromFile(path)
	if err != nil {
		t.Fatalf("failed to load filter: %v", err)
	}
	if len(f.Patterns()) != 1 || f.Patterns()[0] != "password" {
		t.Errorf("expected 1 pattern 'password', got %v", f.Patterns())
	}
}

func TestFilterFileNotFound(t *testing.T) {
	_, err := NewFilterFromFile("/nonexistent/.mcpignore")
	if err == nil {
		t.Errorf("expected error for nonexistent file")
	}
}
