package gitbucket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientUsesGitBucketAPIBaseAndToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/user" {
			t.Fatalf("path = %q, want /api/v3/user", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token secret-token" {
			t.Fatalf("Authorization = %q, want token secret-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"root","email":"root@example.com"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/", "secret-token")
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	user, err := client.CurrentUser(context.Background())
	if err != nil {
		t.Fatalf("CurrentUser returned error: %v", err)
	}
	if user.Login != "root" {
		t.Fatalf("Login = %q, want root", user.Login)
	}
}

func TestListCommitsBuildsPathAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/acme/widgets/commits" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("sha") != "develop" {
			t.Fatalf("sha = %q, want develop", query.Get("sha"))
		}
		if query.Get("author") != "alice" {
			t.Fatalf("author = %q, want alice", query.Get("author"))
		}
		if query.Get("per_page") != "5" {
			t.Fatalf("per_page = %q, want 5", query.Get("per_page"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"sha":"abc123","commit":{"message":"initial commit","author":{"name":"Alice","email":"alice@example.com","date":"2026-06-27T00:00:00Z"}}}]`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	commits, err := client.ListCommits(context.Background(), Repo{Owner: "acme", Name: "widgets"}, ListCommitsOptions{
		Branch: "develop",
		Author: "alice",
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("ListCommits returned error: %v", err)
	}
	if len(commits) != 1 || commits[0].SHA != "abc123" || commits[0].Commit.Message != "initial commit" {
		t.Fatalf("commits = %#v", commits)
	}
}

func TestPutContentEncodesSingleFileCommitRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/api/v3/repos/acme/widgets/contents/docs/readme.md" {
			t.Fatalf("path = %q", r.URL.Path)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["message"] != "docs: update readme" {
			t.Fatalf("message = %q", payload["message"])
		}
		if payload["content"] != "aGVsbG8K" {
			t.Fatalf("content = %q, want base64 hello", payload["content"])
		}
		if payload["branch"] != "develop" {
			t.Fatalf("branch = %q, want develop", payload["branch"])
		}
		if payload["sha"] != "old-sha" {
			t.Fatalf("sha = %q, want old-sha", payload["sha"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"commit":{"sha":"new-sha","html_url":"https://gitbucket/acme/widgets/commit/new-sha"},"content":{"path":"docs/readme.md","sha":"blob-sha"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	result, err := client.PutContent(context.Background(), Repo{Owner: "acme", Name: "widgets"}, "docs/readme.md", PutContentRequest{
		Message: "docs: update readme",
		Content: []byte("hello\n"),
		Branch:  "develop",
		SHA:     "old-sha",
	})
	if err != nil {
		t.Fatalf("PutContent returned error: %v", err)
	}
	if result.Commit.SHA != "new-sha" {
		t.Fatalf("commit sha = %q, want new-sha", result.Commit.SHA)
	}
}

func TestPullRequestMethodsUseGitBucketEndpoints(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch r.Method + " " + r.URL.Path {
		case "POST /api/v3/repos/acme/widgets/pulls":
			var payload CreatePullRequestRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create pr: %v", err)
			}
			if payload.Title != "Add API" || payload.Head != "feature/api" || payload.Base != "main" {
				t.Fatalf("payload = %#v", payload)
			}
			_, _ = w.Write([]byte(`{"number":7,"title":"Add API","state":"open","html_url":"https://gitbucket/acme/widgets/pull/7","head":{"ref":"feature/api"},"base":{"ref":"main"}}`))
		case "GET /api/v3/repos/acme/widgets/pulls":
			if r.URL.Query().Get("state") != "open" {
				t.Fatalf("state = %q, want open", r.URL.Query().Get("state"))
			}
			_, _ = w.Write([]byte(`[{"number":7,"title":"Add API","state":"open"}]`))
		case "GET /api/v3/repos/acme/widgets/pulls/7":
			_, _ = w.Write([]byte(`{"number":7,"title":"Add API","state":"open","head":{"ref":"feature/api"},"base":{"ref":"main"}}`))
		case "PUT /api/v3/repos/acme/widgets/pulls/7/merge":
			_, _ = w.Write([]byte(`{"merged":true,"message":"Pull Request successfully merged"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	repo := Repo{Owner: "acme", Name: "widgets"}

	created, err := client.CreatePullRequest(context.Background(), repo, CreatePullRequestRequest{
		Title: "Add API",
		Body:  "body",
		Head:  "feature/api",
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest returned error: %v", err)
	}
	if created.Number != 7 {
		t.Fatalf("created number = %d, want 7", created.Number)
	}

	listed, err := client.ListPullRequests(context.Background(), repo, ListPullRequestsOptions{State: "open", Limit: 20})
	if err != nil {
		t.Fatalf("ListPullRequests returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].Number != 7 {
		t.Fatalf("listed = %#v", listed)
	}

	viewed, err := client.GetPullRequest(context.Background(), repo, 7)
	if err != nil {
		t.Fatalf("GetPullRequest returned error: %v", err)
	}
	if viewed.Head.Ref != "feature/api" {
		t.Fatalf("head ref = %q, want feature/api", viewed.Head.Ref)
	}

	merged, err := client.MergePullRequest(context.Background(), repo, 7, MergePullRequestRequest{Method: "merge"})
	if err != nil {
		t.Fatalf("MergePullRequest returned error: %v", err)
	}
	if !merged.Merged {
		t.Fatalf("merged = false, want true")
	}

	if got := strings.Join(requests, "\n"); !strings.Contains(got, "PUT /api/v3/repos/acme/widgets/pulls/7/merge") {
		t.Fatalf("requests = %s", got)
	}
}
