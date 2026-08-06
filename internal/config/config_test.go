package config

import "testing"

func TestNew_DefaultValues(t *testing.T) {
	cfg := New()

	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.DataDir != "." {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, ".")
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "http://localhost:8080")
	}
}
