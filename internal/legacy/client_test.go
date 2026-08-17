package legacy

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

func TestURLKeepsContextRootAndQuery(t *testing.T) {
	client, err := NewClient("https://host.example/gitbucket", "root", "secret")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := client.url("/signin"); got != "https://host.example/gitbucket/signin" {
		t.Fatalf("signin = %q", got)
	}
	got := client.url("/acme/widgets/issues?state=open&page=2")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/gitbucket/acme/widgets/issues" {
		t.Fatalf("path = %q", parsed.Path)
	}
	if parsed.Query().Get("state") != "open" || parsed.Query().Get("page") != "2" {
		t.Fatalf("query = %q", parsed.RawQuery)
	}
}

func TestURLWithoutContextRoot(t *testing.T) {
	client, err := NewClient("https://host.example", "root", "secret")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := client.url("/signin"); got != "https://host.example/signin" {
		t.Fatalf("signin = %q", got)
	}
}

func TestURLDoesNotDoubleEscapeCompareHead(t *testing.T) {
	client, err := NewClient("https://host.example/gitbucket", "root", "secret")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	got := client.url(client.repoPath("acme", "widgets", "compare", "main...feature/api") + "?expand=1")
	if strings.Contains(got, "%252F") {
		t.Fatalf("double-escaped slash in %q", got)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("expand") != "1" {
		t.Fatalf("query = %q", parsed.RawQuery)
	}
	wantPath := "/gitbucket/acme/widgets/compare/main...feature%2Fapi"
	if parsed.EscapedPath() != wantPath {
		t.Fatalf("escaped path = %q, want %q", parsed.EscapedPath(), wantPath)
	}
	if strings.Count(parsed.EscapedPath(), "%2F") != 1 {
		t.Fatalf("want exactly one %%2F in %q", parsed.EscapedPath())
	}
}

func TestNewClientClonesCallerAndNeverFollowsRedirects(t *testing.T) {
	server := newLegacyServer(t, "")
	callerJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	followed := false
	caller := &http.Client{
		Jar: callerJar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			followed = true
			return nil
		},
	}

	client, err := NewClient(server.URL, "root", "secret", WithHTTPClient(caller))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.httpClient == caller {
		t.Fatal("legacy client reused the caller's http.Client")
	}
	if caller.Jar != callerJar {
		t.Fatal("NewClient mutated the caller's cookie jar")
	}

	followed = false
	if err := client.SignIn(context.Background()); err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if followed {
		t.Fatal("legacy client followed a redirect through the caller's CheckRedirect")
	}

	followed = false
	if err := caller.CheckRedirect(&http.Request{}, nil); err != nil {
		t.Fatalf("caller CheckRedirect was replaced: %v", err)
	}
	if !followed {
		t.Fatal("caller CheckRedirect no longer follows redirects")
	}
}

func TestSignInUsesContextRoot(t *testing.T) {
	server := newLegacyServer(t, "/gitbucket")
	client := newTestClient(t, server.URL)
	if err := client.SignIn(context.Background()); err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	found := false
	for _, rec := range server.reqs {
		if rec.Method == "GET" && rec.Path == "/gitbucket/signin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("signin GET missed context root: %#v", server.reqs)
	}
}

func TestParseCreatedLocation(t *testing.T) {
	base, err := url.Parse("http://gitbucket.example")
	if err != nil {
		t.Fatal(err)
	}
	n, got, err := parseCreatedLocation(base, "/acme/widgets/issues/8", "acme", "widgets", "issues")
	if err != nil {
		t.Fatal(err)
	}
	if n != 8 || got != "http://gitbucket.example/acme/widgets/issues/8" {
		t.Fatalf("got %d %q", n, got)
	}
}

func TestParseCreatedLocationKeepsContextRoot(t *testing.T) {
	base, err := url.Parse("http://gitbucket.example/gitbucket")
	if err != nil {
		t.Fatal(err)
	}
	n, got, err := parseCreatedLocation(base, "/gitbucket/acme/widgets/issues/8", "acme", "widgets", "issues")
	if err != nil {
		t.Fatal(err)
	}
	if n != 8 || !strings.HasSuffix(got, "/gitbucket/acme/widgets/issues/8") {
		t.Fatalf("got %d %q", n, got)
	}
}
