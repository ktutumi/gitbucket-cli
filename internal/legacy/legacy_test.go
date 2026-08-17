package legacy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type recordedReq struct {
	Method string
	Path   string
	Query  url.Values
}

type legacyFixture struct {
	URL        string
	prefix     string
	issues     map[int]string
	nextID     int
	creates    int
	dropCreate bool
	baseSHA    string
	headSHA    string
	baseBranch string
	headBranch string
	reqs       []recordedReq
	server     *httptest.Server
}

func newLegacyServer(t *testing.T, prefix string) *legacyFixture {
	t.Helper()
	f := &legacyFixture{
		prefix:     strings.TrimRight(prefix, "/"),
		issues:     map[int]string{},
		nextID:     1,
		baseSHA:    strings.Repeat("1", 40),
		headSHA:    strings.Repeat("2", 40),
		baseBranch: "main",
		headBranch: "feature/api",
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	f.URL = f.server.URL + f.prefix
	return f
}

func (f *legacyFixture) Close() {
	f.server.Close()
}

func (f *legacyFixture) loc(p string) string {
	return f.prefix + p
}

func (f *legacyFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.reqs = append(f.reqs, recordedReq{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.Query()})
	path := r.URL.Path
	if f.prefix != "" {
		if path != f.prefix && !strings.HasPrefix(path, f.prefix+"/") {
			http.NotFound(w, r)
			return
		}
		path = strings.TrimPrefix(path, f.prefix)
		if path == "" {
			path = "/"
		}
	}
	switch {
	case path == "/signin":
		f.handleSignIn(w, r)
	case path == "/acme/widgets/issues":
		f.handleIssueList(w, r)
	case path == "/acme/widgets/issues/new":
		f.handleIssueCreate(w, r)
	case strings.HasPrefix(path, "/acme/widgets/issues/_data/"):
		f.handleIssueData(w, r, strings.TrimPrefix(path, "/acme/widgets/issues/_data/"))
	case strings.HasPrefix(path, "/acme/widgets/compare/"):
		f.handleCompare(w, r)
	case path == "/acme/widgets/pulls/new":
		f.handlePRCreate(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *legacyFixture) handleSignIn(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<form action="`+f.loc("/signin")+`" method="post"><input name="userName"><input name="password"></form>`)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session"})
	w.Header().Set("Location", f.loc("/"))
	w.WriteHeader(http.StatusFound)
}

func (f *legacyFixture) handleIssueList(w http.ResponseWriter, r *http.Request) {
	stateFilter := r.URL.Query().Get("state")
	page := r.URL.Query().Get("page")
	var b strings.Builder
	b.WriteString(`<table class="table-issues">`)
	if (page == "" || page == "1") && stateFilter != "closed" {
		for id := range f.issues {
			fmt.Fprintf(&b, `<tr><td><a class="issue-title" href="%s">t</a></td></tr>`, f.loc(fmt.Sprintf("/acme/widgets/issues/%d", id)))
		}
	}
	b.WriteString(`</table>`)
	_, _ = io.WriteString(w, b.String())
}

func (f *legacyFixture) handleIssueCreate(w http.ResponseWriter, r *http.Request) {
	f.creates++
	if f.dropCreate {
		http.Error(w, "lost", http.StatusInternalServerError)
		return
	}
	_ = r.ParseForm()
	id := f.nextID
	f.nextID++
	f.issues[id] = r.PostFormValue("content")
	w.Header().Set("Location", f.loc(fmt.Sprintf("/acme/widgets/issues/%d", id)))
	w.WriteHeader(http.StatusFound)
}

func (f *legacyFixture) handleIssueData(w http.ResponseWriter, r *http.Request, idStr string) {
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.NotFound(w, r)
		return
	}
	body, ok := f.issues[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	fmt.Fprintf(w, `<textarea id="edit-content">%s</textarea>`, body)
}

func (f *legacyFixture) handleCompare(w http.ResponseWriter, r *http.Request) {
	_, _ = io.WriteString(w, `<form action="`+f.loc("/acme/widgets/pulls/new")+`" method="post">
  <input type="hidden" name="targetUserName" value="acme">
  <input type="hidden" name="targetBranch" value="`+f.baseBranch+`">
  <input type="hidden" name="requestUserName" value="acme">
  <input type="hidden" name="requestRepositoryName" value="widgets">
  <input type="hidden" name="requestBranch" value="`+f.headBranch+`">
  <input type="hidden" name="commitIdFrom" value="`+f.baseSHA+`">
  <input type="hidden" name="commitIdTo" value="`+f.headSHA+`">
</form>`)
}

func (f *legacyFixture) handlePRCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Location", f.loc("/acme/widgets/pull/7"))
	w.WriteHeader(http.StatusFound)
}

func (f *legacyFixture) requireQuery(t *testing.T, pathPart, key, want string) {
	t.Helper()
	for _, rec := range f.reqs {
		if strings.Contains(rec.Path, pathPart) && rec.Query.Get(key) == want {
			return
		}
	}
	t.Fatalf("no request containing %q with %s=%q\nreqs=%#v", pathPart, key, want, f.reqs)
}

func newTestClient(t *testing.T, rawURL string) *Client {
	t.Helper()
	client, err := NewClient(rawURL, "root", "secret")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}
