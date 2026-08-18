package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktutumi/gitbucket-cli/internal/config"
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

func TestAuthLoginLegacyMissingUserExitsValidation(t *testing.T) {
	var errOut bytes.Buffer
	code := Run(context.Background(), []string{"auth", "login", "--url", "https://legacy.example.com", "--legacy"}, Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &errOut,
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
	})
	if code != 5 {
		t.Fatalf("exit code = %d, want 5, stderr = %s", code, errOut.String())
	}
}

func TestAuthLoginLegacyRejectsTokenExitsValidation(t *testing.T) {
	var errOut bytes.Buffer
	code := Run(context.Background(), []string{"auth", "login", "--url", "https://legacy.example.com", "--legacy", "--user", "root", "--token", "secret"}, Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &errOut,
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
	})
	if code != 5 {
		t.Fatalf("exit code = %d, want 5, stderr = %s", code, errOut.String())
	}
}

func TestAuthLoginLegacyDoesNotRequireToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"auth", "login", "--url", "https://legacy.example.com", "--legacy", "--user", "root"}, Options{
		Stdout:     &out,
		Stderr:     &errOut,
		ConfigPath: configPath,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "https://legacy.example.com") {
		t.Fatalf("output = %q", out.String())
	}

	cfg, err := config.NewStore(configPath).Load()
	if err != nil {
		t.Fatal(err)
	}
	host := cfg.Hosts["https://legacy.example.com"]
	if !host.Legacy || host.User != "root" || host.Token != "" {
		t.Fatalf("host = %#v", host)
	}
}

func TestAuthStatusLegacySkipsAPI(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.UpsertHost(config.Config{}, "https://legacy.example.com", config.HostConfig{Legacy: true, User: "root"})
	if err := config.NewStore(configPath).Save(cfg); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	code := Run(context.Background(), []string{"auth", "status"}, Options{
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
		ConfigPath: configPath,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "https://legacy.example.com") || !strings.Contains(got, "legacy") || !strings.Contains(got, "root") {
		t.Fatalf("output = %q", got)
	}
	if strings.Contains(strings.ToLower(got), "password") {
		t.Fatalf("status leaked password presence: %q", got)
	}
}

func TestIssueCreateLegacyReusesMarker(t *testing.T) {
	server := newCLILegacyServer(t)
	server.issues[12] = "Asana-Task-ID: 99\n"
	opts := legacyCLIOptions(t, server)

	var out bytes.Buffer
	opts.Stdout = &out
	code := Run(context.Background(), []string{
		"--repo", "acme/widgets", "issue", "create",
		"--title", "Task", "--body", "Asana-Task-ID: 99\n",
		"--marker-label", "Asana-Task-ID", "--dedupe-key", "99",
		"--password", "secret",
	}, opts)
	if code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, out.String())
	}
	if !strings.Contains(out.String(), "#12") {
		t.Fatalf("output = %q", out.String())
	}
	if server.creates != 0 {
		t.Fatalf("posted a new issue: creates = %d", server.creates)
	}
}

func TestIssueCreateLegacyUncertaintySuppressesRepost(t *testing.T) {
	server := newCLILegacyServer(t)
	server.dropCreate = true
	opts := legacyCLIOptions(t, server)

	args := []string{
		"--repo", "acme/widgets", "issue", "create",
		"--title", "Task", "--body", "Asana-Task-ID: 99\n",
		"--marker-label", "Asana-Task-ID", "--dedupe-key", "99",
		"--password", "secret",
	}
	if code := Run(context.Background(), args, opts); code != 2 {
		t.Fatalf("first exit code = %d, want 2", code)
	}
	creates := server.creates
	if code := Run(context.Background(), args, opts); code != 2 {
		t.Fatalf("second exit code = %d, want 2", code)
	}
	if server.creates != creates {
		t.Fatalf("unsafe re-POST: creates %d -> %d", creates, server.creates)
	}
}

func TestPRCreateLegacyMissingTitleExitsValidation(t *testing.T) {
	server := newCLILegacyServer(t)
	opts := legacyCLIOptions(t, server)
	var errOut bytes.Buffer
	opts.Stderr = &errOut
	code := Run(context.Background(), []string{
		"--repo", "acme/widgets", "pr", "create",
		"--base", "main", "--head", "feature/api",
		"--base-sha", strings.Repeat("1", 40), "--head-sha", strings.Repeat("2", 40),
		"--password", "secret",
	}, opts)
	if code != 5 {
		t.Fatalf("exit code = %d, want 5, stderr = %s", code, errOut.String())
	}
}

func TestPRCreateLegacyMissingTitleBeatsBodyConflict(t *testing.T) {
	server := newCLILegacyServer(t)
	opts := legacyCLIOptions(t, server)
	bodyFile := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(bodyFile, []byte("from file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	opts.Stderr = &errOut
	code := Run(context.Background(), []string{
		"--repo", "acme/widgets", "pr", "create",
		"--body", "inline", "--body-file", bodyFile,
		"--base", "main", "--head", "feature/api",
		"--base-sha", strings.Repeat("1", 40), "--head-sha", strings.Repeat("2", 40),
		"--password", "secret",
	}, opts)
	if code != 5 {
		t.Fatalf("exit code = %d, want 5, stderr = %s", code, errOut.String())
	}
}

func TestPRCreateLegacyWhitespaceTitleBeatsBodyConflict(t *testing.T) {
	server := newCLILegacyServer(t)
	opts := legacyCLIOptions(t, server)
	bodyFile := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(bodyFile, []byte("from file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	opts.Stderr = &errOut
	code := Run(context.Background(), []string{
		"--repo", "acme/widgets", "pr", "create",
		"--title", "   ",
		"--body", "inline", "--body-file", bodyFile,
		"--base", "main", "--head", "feature/api",
		"--base-sha", strings.Repeat("1", 40), "--head-sha", strings.Repeat("2", 40),
		"--password", "secret",
	}, opts)
	if code != 5 {
		t.Fatalf("exit code = %d, want 5, stderr = %s", code, errOut.String())
	}
}

func TestPRCreateLegacyRequiresExplicitIdentityFlags(t *testing.T) {
	server := newCLILegacyServer(t)
	opts := legacyCLIOptions(t, server)
	sha1 := strings.Repeat("1", 40)
	sha2 := strings.Repeat("2", 40)

	var errOut bytes.Buffer
	opts.Stderr = &errOut
	code := Run(context.Background(), []string{
		"--repo", "acme/widgets", "pr", "create",
		"--title", "Add API", "--head", "feature/api",
		"--base-sha", sha1, "--head-sha", sha2, "--password", "secret",
	}, opts)
	if code != 5 {
		t.Fatalf("missing --base exit code = %d, want 5, stderr = %s", code, errOut.String())
	}

	errOut.Reset()
	code = Run(context.Background(), []string{
		"--repo", "acme/widgets", "pr", "create",
		"--title", "Add API", "--base", "main",
		"--base-sha", sha1, "--head-sha", sha2, "--password", "secret",
	}, opts)
	if code != 5 {
		t.Fatalf("missing --head exit code = %d, want 5, stderr = %s", code, errOut.String())
	}
}

func TestPRCreateLegacySucceedsWithExplicitIdentity(t *testing.T) {
	server := newCLILegacyServer(t)
	opts := legacyCLIOptions(t, server)
	var out bytes.Buffer
	opts.Stdout = &out
	code := Run(context.Background(), []string{
		"--repo", "acme/widgets", "pr", "create",
		"--title", "Add API", "--body", "Summary",
		"--base", "main", "--head", "feature/api",
		"--base-sha", strings.Repeat("1", 40), "--head-sha", strings.Repeat("2", 40),
		"--password", "secret",
	}, opts)
	if code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, out.String())
	}
	if !strings.Contains(out.String(), "#7") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPRCreateLegacyAcceptsShortIdentityAliases(t *testing.T) {
	server := newCLILegacyServer(t)
	opts := legacyCLIOptions(t, server)
	var out bytes.Buffer
	opts.Stdout = &out
	code := Run(context.Background(), []string{
		"--repo", "acme/widgets", "pr", "create",
		"--title", "Add API", "--body", "Summary",
		"-B", "main", "-H", "feature/api",
		"--base-sha", strings.Repeat("1", 40), "--head-sha", strings.Repeat("2", 40),
		"--password", "secret",
	}, opts)
	if code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, out.String())
	}
	if !strings.Contains(out.String(), "#7") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPRCreateNonLegacyInfersHeadFromCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, "feature/api", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v3/repos/acme/widgets/pulls" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["head"] != "feature/api" || payload["base"] != "main" {
			t.Fatalf("payload = %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"number":7,"title":"Add API","state":"open","html_url":"https://gitbucket/acme/widgets/pull/7","head":{"ref":"feature/api"},"base":{"ref":"main"}}`))
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	code := Run(context.Background(), []string{"--repo", "acme/widgets", "pr", "create", "--title", "Add API", "--base", "main"}, Options{
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
		WorkDir:    dir,
		Env: map[string]string{
			"GITBUCKET_URL":   server.URL,
			"GITBUCKET_TOKEN": "secret-token",
		},
	})
	if code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, out.String())
	}
}

func TestResolveRepoFallsBackToGitRemote(t *testing.T) {
	cases := []struct {
		name   string
		remote string
	}{
		{name: "https", remote: "https://gitbucket.example.com/acme/widgets.git"},
		{name: "scp", remote: "git@gitbucket.example.com:acme/widgets.git"},
		{name: "https-context-root", remote: "https://gitbucket.example.com/gitbucket/acme/widgets.git"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initGitRepo(t, dir, "main", tc.remote)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v3/repos/acme/widgets/commits" {
					t.Fatalf("path = %q, remote = %q", r.URL.Path, tc.remote)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			}))
			t.Cleanup(server.Close)

			code := Run(context.Background(), []string{"commit", "list"}, Options{
				Stdout:     &bytes.Buffer{},
				Stderr:     &bytes.Buffer{},
				ConfigPath: filepath.Join(t.TempDir(), "config.json"),
				WorkDir:    dir,
				Env: map[string]string{
					"GITBUCKET_URL":   server.URL,
					"GITBUCKET_TOKEN": "secret-token",
				},
			})
			if code != 0 {
				t.Fatalf("exit code = %d", code)
			}
		})
	}
}

func TestParseRepoFromRemoteURL(t *testing.T) {
	cases := []struct {
		in          string
		owner, name string
	}{
		{"acme/widgets", "acme", "widgets"},
		{"https://gitbucket.example.com/acme/widgets.git", "acme", "widgets"},
		{"https://gitbucket.example.com/gitbucket/acme/widgets.git", "acme", "widgets"},
		{"git@gitbucket.example.com:acme/widgets.git", "acme", "widgets"},
		{"ssh://git@gitbucket.example.com/acme/widgets.git", "acme", "widgets"},
		{"git@gitbucket.example.com:gitbucket/acme/widgets.git", "acme", "widgets"},
	}
	for _, tc := range cases {
		got, err := parseRepo(tc.in)
		if err != nil {
			t.Fatalf("parseRepo(%q): %v", tc.in, err)
		}
		if got.Owner != tc.owner || got.Name != tc.name {
			t.Fatalf("parseRepo(%q) = %+v, want %s/%s", tc.in, got, tc.owner, tc.name)
		}
	}
}

func TestPRCreateNonLegacyRejectsSHAFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(server.Close)

	var errOut bytes.Buffer
	code := Run(context.Background(), []string{
		"--repo", "acme/widgets", "pr", "create",
		"--title", "Add API", "--head", "feature/api", "--base", "main",
		"--base-sha", strings.Repeat("1", 40),
	}, Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &errOut,
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
		Env: map[string]string{
			"GITBUCKET_URL":   server.URL,
			"GITBUCKET_TOKEN": "secret-token",
		},
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1, stderr = %s", code, errOut.String())
	}
}

func TestLegacyUnsupportedCommand(t *testing.T) {
	server := newCLILegacyServer(t)
	opts := legacyCLIOptions(t, server)
	code := Run(context.Background(), []string{"--repo", "acme/widgets", "commit", "list"}, opts)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestLegacyMissingPasswordIsAuthFailure(t *testing.T) {
	server := newCLILegacyServer(t)
	opts := legacyCLIOptions(t, server)
	opts.Env = map[string]string{"GITBUCKET_URL": server.URL, "GITBUCKET_PASSWORD": ""}
	code := Run(context.Background(), []string{
		"--repo", "acme/widgets", "issue", "create",
		"--title", "Task", "--body", "Asana-Task-ID: 99\n",
		"--marker-label", "Asana-Task-ID", "--dedupe-key", "99",
	}, opts)
	if code != 4 {
		t.Fatalf("exit code = %d, want 4", code)
	}
}

type cliLegacyServer struct {
	URL        string
	issues     map[int]string
	nextID     int
	creates    int
	dropCreate bool
	reqs       []string
}

func newCLILegacyServer(t *testing.T) *cliLegacyServer {
	t.Helper()
	f := &cliLegacyServer{issues: map[int]string{}, nextID: 1}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

func (f *cliLegacyServer) serve(w http.ResponseWriter, r *http.Request) {
	f.reqs = append(f.reqs, r.Method+" "+r.URL.EscapedPath())
	switch {
	case r.URL.Path == "/signin":
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Location", "/")
		w.WriteHeader(http.StatusFound)
	case r.URL.Path == "/acme/widgets/issues":
		page := r.URL.Query().Get("page")
		state := r.URL.Query().Get("state")
		var b strings.Builder
		b.WriteString(`<table class="table-issues">`)
		if (page == "" || page == "1") && state != "closed" {
			for id := range f.issues {
				fmt.Fprintf(&b, `<tr><td><a class="issue-title" href="/acme/widgets/issues/%d">t</a></td></tr>`, id)
			}
		}
		b.WriteString(`</table>`)
		_, _ = io.WriteString(w, b.String())
	case r.URL.Path == "/acme/widgets/issues/new":
		f.creates++
		if f.dropCreate {
			http.Error(w, "lost", http.StatusInternalServerError)
			return
		}
		_ = r.ParseForm()
		id := f.nextID
		f.nextID++
		f.issues[id] = r.PostFormValue("content")
		w.Header().Set("Location", fmt.Sprintf("/acme/widgets/issues/%d", id))
		w.WriteHeader(http.StatusFound)
	case strings.HasPrefix(r.URL.Path, "/acme/widgets/issues/_data/"):
		var id int
		_, _ = fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/acme/widgets/issues/_data/"), "%d", &id)
		fmt.Fprintf(w, `<textarea id="edit-content">%s</textarea>`, f.issues[id])
	case strings.HasPrefix(r.URL.Path, "/acme/widgets/compare/"):
		_, _ = io.WriteString(w, `<form action="/acme/widgets/pulls/new" method="post">
  <input type="hidden" name="targetUserName" value="acme">
  <input type="hidden" name="targetBranch" value="main">
  <input type="hidden" name="requestUserName" value="acme">
  <input type="hidden" name="requestRepositoryName" value="widgets">
  <input type="hidden" name="requestBranch" value="feature/api">
  <input type="hidden" name="commitIdFrom" value="`+strings.Repeat("1", 40)+`">
  <input type="hidden" name="commitIdTo" value="`+strings.Repeat("2", 40)+`">
</form>`)
	case r.URL.Path == "/acme/widgets/pulls/new":
		w.Header().Set("Location", "/acme/widgets/pull/7")
		w.WriteHeader(http.StatusFound)
	default:
		http.NotFound(w, r)
	}
}

func legacyCLIOptions(t *testing.T, server *cliLegacyServer) Options {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.UpsertHost(config.Config{}, server.URL, config.HostConfig{Legacy: true, User: "root"})
	if err := config.NewStore(configPath).Save(cfg); err != nil {
		t.Fatal(err)
	}
	return Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		ConfigPath: configPath,
		StateDir:   t.TempDir(),
		Env:        map[string]string{"GITBUCKET_URL": server.URL},
	}
}

func initGitRepo(t *testing.T, dir, branch, remote string) {
	t.Helper()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runGit("init", "-b", branch)
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README.md")
	runGit("commit", "-m", "init")
	if remote != "" {
		runGit("remote", "add", "origin", remote)
	}
}
