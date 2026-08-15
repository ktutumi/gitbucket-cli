package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthLoginStatusAndLogout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/user" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "token secret-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"root"}`))
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")

	var out bytes.Buffer
	code := Run(context.Background(), []string{"auth", "login", "--url", server.URL, "--token", "secret-token"}, Options{
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
		ConfigPath: configPath,
	})
	if code != 0 {
		t.Fatalf("login exit code = %d, stderr/stdout = %s", code, out.String())
	}
	if !strings.Contains(out.String(), "Logged in to "+server.URL) {
		t.Fatalf("login output = %q", out.String())
	}

	out.Reset()
	code = Run(context.Background(), []string{"auth", "status"}, Options{
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
		ConfigPath: configPath,
	})
	if code != 0 {
		t.Fatalf("status exit code = %d, output = %s", code, out.String())
	}
	if !strings.Contains(out.String(), "root") {
		t.Fatalf("status output = %q", out.String())
	}

	out.Reset()
	code = Run(context.Background(), []string{"auth", "logout", "--url", server.URL}, Options{
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
		ConfigPath: configPath,
	})
	if code != 0 {
		t.Fatalf("logout exit code = %d", code)
	}
	if !strings.Contains(out.String(), "Logged out from "+server.URL) {
		t.Fatalf("logout output = %q", out.String())
	}
}

func TestCommitListJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/acme/widgets/commits" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("sha") != "main" {
			t.Fatalf("sha = %q", r.URL.Query().Get("sha"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"sha":"abc123","commit":{"message":"initial commit","author":{"name":"Alice","email":"alice@example.com","date":"2026-06-27T00:00:00Z"}}}]`))
	}))
	defer server.Close()

	var out bytes.Buffer
	code := Run(context.Background(), []string{"--repo", "acme/widgets", "commit", "list", "--branch", "main", "--json"}, Options{
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
		Env: map[string]string{
			"GITBUCKET_URL":   server.URL,
			"GITBUCKET_TOKEN": "secret-token",
		},
	})
	if code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, out.String())
	}

	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if len(rows) != 1 || rows[0]["sha"] != "abc123" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestCommitCreateReadsContentFileAndPrintsCommitURL(t *testing.T) {
	contentPath := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(contentPath, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write content: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v3/repos/acme/widgets/contents/docs/note.txt" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/api/v3/repos/acme/widgets/contents/docs/note.txt" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["message"] != "docs: add note" {
			t.Fatalf("message = %q", payload["message"])
		}
		if payload["content"] != "aGVsbG8K" {
			t.Fatalf("content = %q", payload["content"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"commit":{"sha":"abc123","html_url":"https://gitbucket/acme/widgets/commit/abc123"},"content":{"path":"docs/note.txt"}}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	code := Run(context.Background(), []string{"--repo", "acme/widgets", "commit", "create", "--message", "docs: add note", "--path", "docs/note.txt", "--content-file", contentPath}, Options{
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
		Env: map[string]string{
			"GITBUCKET_URL":   server.URL,
			"GITBUCKET_TOKEN": "secret-token",
		},
	})
	if code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, out.String())
	}
	if !strings.Contains(out.String(), "abc123") || !strings.Contains(out.String(), "https://gitbucket/acme/widgets/commit/abc123") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPRCreateAndView(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v3/repos/acme/widgets/pulls":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if payload["title"] != "Add API" || payload["head"] != "feature/api" || payload["base"] != "main" {
				t.Fatalf("payload = %#v", payload)
			}
			_, _ = w.Write([]byte(`{"number":7,"title":"Add API","state":"open","html_url":"https://gitbucket/acme/widgets/pull/7","head":{"ref":"feature/api"},"base":{"ref":"main"}}`))
		case "GET /api/v3/repos/acme/widgets/pulls/7":
			_, _ = w.Write([]byte(`{"number":7,"title":"Add API","state":"open","html_url":"https://gitbucket/acme/widgets/pull/7","head":{"ref":"feature/api"},"base":{"ref":"main"}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	opts := Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
		Env: map[string]string{
			"GITBUCKET_URL":   server.URL,
			"GITBUCKET_TOKEN": "secret-token",
		},
	}

	var out bytes.Buffer
	opts.Stdout = &out
	code := Run(context.Background(), []string{"--repo", "acme/widgets", "pr", "create", "--title", "Add API", "--body", "body", "--head", "feature/api", "--base", "main"}, opts)
	if code != 0 {
		t.Fatalf("create exit code = %d, output = %s", code, out.String())
	}
	if !strings.Contains(out.String(), "https://gitbucket/acme/widgets/pull/7") {
		t.Fatalf("create output = %q", out.String())
	}

	out.Reset()
	code = Run(context.Background(), []string{"--repo", "acme/widgets", "pr", "view", "7", "--json"}, opts)
	if code != 0 {
		t.Fatalf("view exit code = %d, output = %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"number": 7`) {
		t.Fatalf("view output = %q", out.String())
	}
}
