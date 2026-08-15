package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTripWritesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewStore(path)

	cfg := Config{
		DefaultURL: "https://gitbucket.example.com",
		Hosts: map[string]HostConfig{
			"https://gitbucket.example.com": {Token: "secret-token"},
		},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %v, want 0600", got)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.DefaultURL != cfg.DefaultURL {
		t.Fatalf("DefaultURL = %q", loaded.DefaultURL)
	}
	if loaded.Hosts[cfg.DefaultURL].Token != "secret-token" {
		t.Fatalf("token was not round-tripped")
	}
}

func TestResolveEnvironmentOverridesConfig(t *testing.T) {
	cfg := Config{
		DefaultURL: "https://from-config.example.com",
		Hosts: map[string]HostConfig{
			"https://from-config.example.com": {Token: "config-token"},
		},
	}
	env := map[string]string{
		"GITBUCKET_URL":   "https://from-env.example.com/",
		"GITBUCKET_TOKEN": "env-token",
	}

	resolved, err := Resolve(cfg, env, "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.URL != "https://from-env.example.com" {
		t.Fatalf("URL = %q, want trimmed env URL", resolved.URL)
	}
	if resolved.Token != "env-token" {
		t.Fatalf("Token = %q, want env-token", resolved.Token)
	}
}

func TestResolveUsesExplicitURLAndMatchingHostToken(t *testing.T) {
	cfg := Config{
		DefaultURL: "https://default.example.com",
		Hosts: map[string]HostConfig{
			"https://gitbucket.example.com": {Token: "host-token"},
		},
	}

	resolved, err := Resolve(cfg, nil, "https://gitbucket.example.com/")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.URL != "https://gitbucket.example.com" {
		t.Fatalf("URL = %q", resolved.URL)
	}
	if resolved.Token != "host-token" {
		t.Fatalf("Token = %q, want host-token", resolved.Token)
	}
}
