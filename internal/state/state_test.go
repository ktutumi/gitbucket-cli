package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeURLEquivalence(t *testing.T) {
	normalized := func(raw string) string {
		t.Helper()
		got, err := NormalizeURL(raw)
		if err != nil {
			t.Fatalf("NormalizeURL(%q): %v", raw, err)
		}
		return got
	}

	want := "http://gitbucket.example/"
	variants := []string{
		"HTTP://GITBUCKET.EXAMPLE:80/",
		"http://gitbucket.example:80",
		"http://gitbucket.example/",
		"http://gitbucket.example//",
		"http://gitbucket.example///",
		"http://gitbucket.example/?query=1#frag",
	}
	for _, v := range variants {
		if got := normalized(v); got != want {
			t.Fatalf("NormalizeURL(%q) = %q, want %q", v, got, want)
		}
	}
	if got := normalized("https://gitbucket.example:443/"); got != "https://gitbucket.example/" {
		t.Fatalf("https default port: %q", got)
	}
	if got := normalized("https://gitbucket.example:8080/gitbucket///app/"); got != "https://gitbucket.example:8080/gitbucket/app" {
		t.Fatalf("non-default port + repeated and trailing slashes: %q", got)
	}
	if got := normalized("https://[2001:db8::1]:443/"); got != "https://[2001:db8::1]/" {
		t.Fatalf("IPv6 default port: %q", got)
	}
	if got := normalized("https://[2001:db8::1]:8080/"); got != "https://[2001:db8::1]:8080/" {
		t.Fatalf("IPv6 non-default port: %q", got)
	}
}

func TestNormalizeURLRejectsInvalid(t *testing.T) {
	for _, raw := range []string{
		"",
		"   ",
		"gitbucket.example",
		"ftp://gitbucket.example/",
		"http:///path",
	} {
		if _, err := NormalizeURL(raw); !errors.Is(err, ErrInvalidURL) {
			t.Fatalf("NormalizeURL(%q) = %v, want ErrInvalidURL", raw, err)
		}
	}
}

func TestKeyUnambiguousFieldBoundaries(t *testing.T) {
	base := Identity{URL: "https://gitbucket.example/", MarkerLabel: "Asana-Task-ID", DedupeKey: "123"}
	a, err := Key(Identity{URL: base.URL, Owner: "ab", Repo: "c", MarkerLabel: base.MarkerLabel, DedupeKey: base.DedupeKey})
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	b, err := Key(Identity{URL: base.URL, Owner: "a", Repo: "bc", MarkerLabel: base.MarkerLabel, DedupeKey: base.DedupeKey})
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	if a == b {
		t.Fatalf("keys collide across field boundaries: %s", a)
	}
}

func TestKeyIncludesMarkerLabel(t *testing.T) {
	base := Identity{URL: "https://gitbucket.example/", Owner: "o", Repo: "r", DedupeKey: "123"}
	a, err := Key(Identity{URL: base.URL, Owner: base.Owner, Repo: base.Repo, MarkerLabel: "Asana-Task-ID", DedupeKey: base.DedupeKey})
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	b, err := Key(Identity{URL: base.URL, Owner: base.Owner, Repo: base.Repo, MarkerLabel: "Other-Label", DedupeKey: base.DedupeKey})
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	if a == b {
		t.Fatalf("keys collide across marker labels: %s", a)
	}
}

func TestKeyEquivalentURLsShare(t *testing.T) {
	id := func(url string) Identity {
		return Identity{URL: url, Owner: "o", Repo: "r", MarkerLabel: "L", DedupeKey: "123"}
	}
	a, err := Key(id("HTTP://GITBUCKET.EXAMPLE:80/"))
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	b, err := Key(id("http://gitbucket.example"))
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	if a != b {
		t.Fatalf("equivalent URLs produced different keys")
	}
}

func TestKeyRejectsInvalidIdentity(t *testing.T) {
	for _, id := range []Identity{
		{URL: "https://gitbucket.example/", Owner: "", Repo: "r", MarkerLabel: "L", DedupeKey: "k"},
		{URL: "https://gitbucket.example/", Owner: "o", Repo: "", MarkerLabel: "L", DedupeKey: "k"},
		{URL: "https://gitbucket.example/", Owner: "o", Repo: "r", MarkerLabel: "", DedupeKey: "k"},
		{URL: "https://gitbucket.example/", Owner: "o", Repo: "r", MarkerLabel: "L", DedupeKey: ""},
	} {
		if _, err := Key(id); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("Key(%v) = %v, want ErrInvalidKey", id, err)
		}
	}
}

func TestLockAcquireRelease(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), "state"))
	key, err := Key(Identity{URL: "https://gitbucket.example/", Owner: "o", Repo: "r", MarkerLabel: "L", DedupeKey: "k"})
	if err != nil {
		t.Fatalf("Key: %v", err)
	}

	lease, err := manager.Acquire(key)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := manager.Acquire(key); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Acquire = %v, want ErrBusy", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("double Release must be a no-op: %v", err)
	}
	lease, err = manager.Acquire(key)
	if err != nil {
		t.Fatalf("re-Acquire after Release: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestLockRejectsInvalidKey(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), "state"))
	uppercase := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	nonHex := "gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg"
	for _, key := range []string{
		"not-hex",
		uppercase,
		nonHex,
		"000000000000000000000000000000000000000000000000000000000000000",
		"../",
		"",
	} {
		lease, err := manager.Acquire(key)
		_ = lease
		if !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("Acquire(%q) = %v, want ErrInvalidKey", key, err)
		}
	}
}

func TestStateDirMode0700(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	manager := NewManager(root)
	rootPath, err := manager.Root()
	_ = rootPath
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("root mode = %v, want 0700", got)
	}
}

func TestStateDirTightensPermissiveRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manager := NewManager(root)
	if _, err := manager.Root(); err != nil {
		t.Fatalf("Root: %v", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("root mode = %v, want 0700 after tightening", got)
	}
}

func TestStateDirRejectsSymlinkRoot(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	if err := os.MkdirAll(realRoot, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	linkRoot := filepath.Join(base, "link")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	manager := NewManager(linkRoot)
	rootPath, err := manager.Root()
	_ = rootPath
	if err == nil {
		t.Fatalf("Root with symlink root must fail")
	}
}

func TestUncertaintyRoundTrip(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), "state"))
	key, err := Key(Identity{URL: "https://gitbucket.example/", Owner: "o", Repo: "r", MarkerLabel: "L", DedupeKey: "k"})
	if err != nil {
		t.Fatalf("Key: %v", err)
	}

	if _, err := manager.ReadUncertainty(key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ReadUncertainty on empty = %v, want ErrNotFound", err)
	}

	record := Uncertainty{Token: "t", Phase: "posting", IssueNumber: 104}
	if err := manager.WriteUncertainty(key, record); err != nil {
		t.Fatalf("WriteUncertainty: %v", err)
	}

	path, err := manager.path(key, ".uncertain.json")
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat marker: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("marker mode = %v, want 0600", got)
	}

	loaded, err := manager.ReadUncertainty(key)
	if err != nil {
		t.Fatalf("ReadUncertainty: %v", err)
	}
	if loaded.Token != record.Token || loaded.Phase != record.Phase || loaded.IssueNumber != record.IssueNumber {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}

	if err := manager.ClearUncertainty(key); err != nil {
		t.Fatalf("ClearUncertainty: %v", err)
	}
	if _, err := manager.ReadUncertainty(key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ReadUncertainty after clear = %v, want ErrNotFound", err)
	}
}
