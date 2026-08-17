package legacy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ktutumi/gitbucket-cli/internal/state"
)

func TestCreateIssueReusesExistingMarker(t *testing.T) {
	server := newLegacyServer(t, "")
	server.issues[12] = "Asana-Task-ID: 99\n"
	server.nextID = 13

	client := newTestClient(t, server.URL)
	got, err := client.CreateIssue(context.Background(), state.NewManager(t.TempDir()), CreateIssueRequest{
		Owner:       "acme",
		Repo:        "widgets",
		Title:       "Task",
		Body:        "Asana-Task-ID: 99\n",
		MarkerLabel: "Asana-Task-ID",
		DedupeKey:   "99",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v\nreqs=%#v\nissues=%#v", err, server.reqs, server.issues)
	}
	if !got.Reused || got.Number != 12 {
		t.Fatalf("result = %#v", got)
	}
	server.requireQuery(t, "/acme/widgets/issues", "state", "open")
	server.requireQuery(t, "/acme/widgets/issues", "page", "1")
	server.requireQuery(t, "/issues/_data/12", "dataType", "html")
}

func TestCreateIssuePostsOnceAndSuppressesUncertainty(t *testing.T) {
	server := newLegacyServer(t, "")
	mgr := state.NewManager(t.TempDir())
	client := newTestClient(t, server.URL)
	req := CreateIssueRequest{
		Owner:       "acme",
		Repo:        "widgets",
		Title:       "Task",
		Body:        "Asana-Task-ID: 99\n",
		MarkerLabel: "Asana-Task-ID",
		DedupeKey:   "99",
	}
	got, err := client.CreateIssue(context.Background(), mgr, req)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if got.Reused || got.Number == 0 {
		t.Fatalf("result = %#v", got)
	}

	again, err := client.CreateIssue(context.Background(), mgr, req)
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if !again.Reused || again.Number != got.Number {
		t.Fatalf("reuse = %#v", again)
	}
}

func TestCreateIssueUncertaintySuppressesRepost(t *testing.T) {
	server := newLegacyServer(t, "")
	server.dropCreate = true
	mgr := state.NewManager(t.TempDir())
	client := newTestClient(t, server.URL)
	req := CreateIssueRequest{
		Owner:       "acme",
		Repo:        "widgets",
		Title:       "Task",
		Body:        "Asana-Task-ID: 99\n",
		MarkerLabel: "Asana-Task-ID",
		DedupeKey:   "99",
	}
	if _, err := client.CreateIssue(context.Background(), mgr, req); !errors.Is(err, ErrUncertainty) {
		t.Fatalf("first create err = %v, want ErrUncertainty", err)
	}
	creates := server.creates
	if _, err := client.CreateIssue(context.Background(), mgr, req); !errors.Is(err, ErrUncertainty) {
		t.Fatalf("second create err = %v, want ErrUncertainty", err)
	}
	if server.creates != creates {
		t.Fatalf("unsafe re-POST: creates %d -> %d", creates, server.creates)
	}
}

func TestCreateIssueLostResponseMultipleMatchesIsAmbiguous(t *testing.T) {
	server := newLegacyServer(t, "")
	server.dropCreate = true
	server.dropInserts = 2
	client := newTestClient(t, server.URL)
	_, err := client.CreateIssue(context.Background(), state.NewManager(t.TempDir()), CreateIssueRequest{
		Owner:       "acme",
		Repo:        "widgets",
		Title:       "Task",
		Body:        "Asana-Task-ID: 99\n",
		MarkerLabel: "Asana-Task-ID",
		DedupeKey:   "99",
	})
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("err = %v, want ErrAmbiguous", err)
	}
}

func TestCreateIssuePostCreateMultipleMatchesIsAmbiguous(t *testing.T) {
	server := newLegacyServer(t, "")
	server.extraCreates = 1
	client := newTestClient(t, server.URL)
	_, err := client.CreateIssue(context.Background(), state.NewManager(t.TempDir()), CreateIssueRequest{
		Owner:       "acme",
		Repo:        "widgets",
		Title:       "Task",
		Body:        "Asana-Task-ID: 99\n",
		MarkerLabel: "Asana-Task-ID",
		DedupeKey:   "99",
	})
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("err = %v, want ErrAmbiguous", err)
	}
}

func TestCreateIssueResultJSONKeys(t *testing.T) {
	data, err := json.Marshal(CreateIssueResult{Number: 12, URL: "https://gitbucket/acme/widgets/issues/12", Reused: true})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"number":   float64(12),
		"html_url": "https://gitbucket/acme/widgets/issues/12",
		"reused":   true,
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

func TestCreateIssueOverrideAllowsZeroMatchPost(t *testing.T) {
	server := newLegacyServer(t, "")
	server.dropCreate = true
	mgr := state.NewManager(t.TempDir())
	client := newTestClient(t, server.URL)
	req := CreateIssueRequest{
		Owner:       "acme",
		Repo:        "widgets",
		Title:       "Task",
		Body:        "Asana-Task-ID: 99\n",
		MarkerLabel: "Asana-Task-ID",
		DedupeKey:   "99",
	}
	if _, err := client.CreateIssue(context.Background(), mgr, req); !errors.Is(err, ErrUncertainty) {
		t.Fatalf("seed uncertainty: %v", err)
	}
	server.dropCreate = false
	req.OverrideUncertainty = true
	got, err := client.CreateIssue(context.Background(), mgr, req)
	if err != nil {
		t.Fatalf("override: %v", err)
	}
	if got.Reused {
		t.Fatalf("override reused unexpectedly: %#v", got)
	}
}

func TestCreateIssueCanonicalizesMarkerBeforeKey(t *testing.T) {
	server := newLegacyServer(t, "")
	server.dropCreate = true
	mgr := state.NewManager(t.TempDir())
	client := newTestClient(t, server.URL)
	spaced := CreateIssueRequest{
		Owner:       "acme",
		Repo:        "widgets",
		Title:       "Task",
		Body:        "Asana-Task-ID: 99\n",
		MarkerLabel: " Asana-Task-ID ",
		DedupeKey:   "99 ",
	}
	if _, err := client.CreateIssue(context.Background(), mgr, spaced); !errors.Is(err, ErrUncertainty) {
		t.Fatalf("spaced create err = %v, want ErrUncertainty", err)
	}
	creates := server.creates
	trimmed := spaced
	trimmed.MarkerLabel = "Asana-Task-ID"
	trimmed.DedupeKey = "99"
	if _, err := client.CreateIssue(context.Background(), mgr, trimmed); !errors.Is(err, ErrUncertainty) {
		t.Fatalf("trimmed create err = %v, want ErrUncertainty", err)
	}
	if server.creates != creates {
		t.Fatalf("whitespace variants used different keys: creates %d -> %d", creates, server.creates)
	}
}

func TestCreateIssueRejectsMissingMarker(t *testing.T) {
	client := newTestClient(t, "http://example.invalid")
	_, err := client.CreateIssue(context.Background(), state.NewManager(t.TempDir()), CreateIssueRequest{
		Owner:       "acme",
		Repo:        "widgets",
		Title:       "Task",
		Body:        "no marker\n",
		MarkerLabel: "Asana-Task-ID",
		DedupeKey:   "99",
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestCreateIssueKeepsContextRoot(t *testing.T) {
	server := newLegacyServer(t, "/gitbucket")
	client := newTestClient(t, server.URL)
	got, err := client.CreateIssue(context.Background(), state.NewManager(t.TempDir()), CreateIssueRequest{
		Owner:       "acme",
		Repo:        "widgets",
		Title:       "Task",
		Body:        "Asana-Task-ID: 99\n",
		MarkerLabel: "Asana-Task-ID",
		DedupeKey:   "99",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if got.Number == 0 || !stringsHasPrefix(got.URL, server.URL+"/acme/widgets/issues/") {
		t.Fatalf("result = %#v", got)
	}
	server.requireQuery(t, "/gitbucket/acme/widgets/issues", "state", "open")
	server.requireQuery(t, "/gitbucket/acme/widgets/issues", "page", "1")
	server.requireQuery(t, "/issues/_data/", "dataType", "html")
}

func stringsHasPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}
