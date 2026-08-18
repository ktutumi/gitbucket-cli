package legacy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCreatePullRequestValidatesCompareIdentity(t *testing.T) {
	server := newLegacyServer(t, "")
	client := newTestClient(t, server.URL)
	got, err := client.CreatePullRequest(context.Background(), CreatePullRequestRequest{
		Owner:   "acme",
		Repo:    "widgets",
		Title:   "Add API",
		Body:    "Summary",
		Base:    "main",
		Head:    "feature/api",
		BaseSHA: strings.Repeat("1", 40),
		HeadSHA: strings.Repeat("2", 40),
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if got.Number != 7 {
		t.Fatalf("result = %#v", got)
	}
	server.requireQuery(t, "/compare/", "expand", "1")
}

func TestCreatePullRequestRejectsIdentityMismatch(t *testing.T) {
	server := newLegacyServer(t, "")
	client := newTestClient(t, server.URL)
	_, err := client.CreatePullRequest(context.Background(), CreatePullRequestRequest{
		Owner:   "acme",
		Repo:    "widgets",
		Title:   "Add API",
		Base:    "main",
		Head:    "feature/api",
		BaseSHA: strings.Repeat("a", 40),
		HeadSHA: strings.Repeat("b", 40),
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestCreatePullRequestKeepsContextRoot(t *testing.T) {
	server := newLegacyServer(t, "/gitbucket")
	client := newTestClient(t, server.URL)
	got, err := client.CreatePullRequest(context.Background(), CreatePullRequestRequest{
		Owner:   "acme",
		Repo:    "widgets",
		Title:   "Add API",
		Base:    "main",
		Head:    "feature/api",
		BaseSHA: strings.Repeat("1", 40),
		HeadSHA: strings.Repeat("2", 40),
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if !strings.Contains(got.URL, "/gitbucket/acme/widgets/pull/7") {
		t.Fatalf("url = %q", got.URL)
	}
	server.requireQuery(t, "/gitbucket/acme/widgets/compare/", "expand", "1")
	var comparePath string
	for _, rec := range server.reqs {
		if strings.Contains(rec.Path, "/compare/") {
			comparePath = rec.Path
			break
		}
	}
	if comparePath == "" {
		t.Fatalf("compare request missing: %#v", server.reqs)
	}
	if strings.Contains(comparePath, "%252F") {
		t.Fatalf("double-escaped compare path %q", comparePath)
	}
	if !strings.Contains(comparePath, "feature%2Fapi") {
		t.Fatalf("compare path must keep one %%2F: %q", comparePath)
	}
	if strings.Contains(comparePath, "feature/api") {
		t.Fatalf("compare path split feature/api into extra segments: %q", comparePath)
	}
}

func TestCreatePullRequestResultJSONKeys(t *testing.T) {
	data, err := json.Marshal(CreatePullRequestResult{Number: 7, URL: "https://gitbucket/acme/widgets/pull/7"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"number":   float64(7),
		"html_url": "https://gitbucket/acme/widgets/pull/7",
	}
	if len(got) != len(want) {
		t.Fatalf("keys = %v, want %v; payload = %s", got, want, data)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("json[%q] = %#v, want %#v; payload = %s", key, got[key], value, data)
		}
	}
	for _, leaked := range []string{"Number", "URL", "Reused"} {
		if _, ok := got[leaked]; ok {
			t.Fatalf("exported Go name %q leaked: %s", leaked, data)
		}
	}
}
