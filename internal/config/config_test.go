package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.JWTTTL != 24*time.Hour {
		t.Errorf("JWTTTL = %v, want 24h", cfg.JWTTTL)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	os.Setenv("CHATROOM_HTTP_ADDR", ":9090")
	defer os.Unsetenv("CHATROOM_HTTP_ADDR")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want :9090 from env override", cfg.HTTPAddr)
	}
}
