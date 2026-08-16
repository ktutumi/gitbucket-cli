// Package state provides the durable local state for idempotent legacy
// Issue creation: canonical host keys, an OS-level lock, and an atomic,
// crash-safe uncertainty record that prevents re-POSTing after a lost
// response.
//
// The state directory is private (0700) and uncertainty records are written
// to a 0600 temp file in the same directory, synced, then renamed into
// place so a crash never leaves a partially written marker.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidURL = errors.New("state: invalid URL")
	ErrInvalidKey = errors.New("state: invalid key")
	ErrBusy       = errors.New("state: lock is busy")
	ErrNotFound   = errors.New("state: uncertainty record not found")
)

// NormalizeURL canonicalizes a legacy-host base URL for use as the first
// key field. It lowercases the scheme and host, drops the default port,
// strips userinfo, query, and fragment, collapses duplicate slashes in the
// path, trims a trailing slash, and represents an empty path as "/".
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidURL
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalidURL
	}
	if parsed.Scheme == "" {
		return "", ErrInvalidURL
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrInvalidURL
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", ErrInvalidURL
	}
	// An IPv6 literal host contains ':' and must be bracketed in a URL.
	hostForURL := host
	if strings.Contains(host, ":") {
		hostForURL = "[" + host + "]"
	}
	netloc := hostForURL
	if port := parsed.Port(); port != "" {
		if port != defaultPort(scheme) {
			netloc = net.JoinHostPort(host, port)
		}
	}

	path := parsed.EscapedPath()
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}
	return scheme + "://" + netloc + path, nil
}

func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

// Identity is the dedupe identity of one idempotent Issue create.
type Identity struct {
	URL         string
	Owner       string
	Repo        string
	MarkerLabel string
	DedupeKey   string
}

// Key returns a 64-character lowercase hex key unique to the identity.
// The preimage is a JSON array, so field boundaries are unambiguous even
// when values share prefixes (for example Owner "ab"/Repo "c" vs
// Owner "a"/Repo "bc").
func Key(id Identity) (string, error) {
	urlValue, err := NormalizeURL(id.URL)
	if err != nil {
		return "", err
	}
	if id.Owner == "" || id.Repo == "" || id.MarkerLabel == "" || id.DedupeKey == "" {
		return "", ErrInvalidKey
	}
	preimage, err := json.Marshal([]string{
		urlValue,
		id.Owner,
		id.Repo,
		id.MarkerLabel,
		id.DedupeKey,
	})
	if err != nil {
		return "", ErrInvalidKey
	}
	sum := sha256.Sum256(preimage)
	return hex.EncodeToString(sum[:]), nil
}

func validKey(key string) error {
	if len(key) != 64 {
		return ErrInvalidKey
	}
	for _, c := range key {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ErrInvalidKey
		}
	}
	return nil
}

// Manager owns the state directory.
type Manager struct {
	root string
}

// NewManager returns a Manager rooted at root. An empty root defaults to
// ~/.cache/gitbucket-cli.
func NewManager(root string) Manager {
	if root == "" {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			root = filepath.Join(home, ".cache", "gitbucket-cli")
		}
	}
	return Manager{root: root}
}

// Root returns the state directory. The directory is created with mode 0700
// when first needed, and an existing root is verified to be a real
// directory (not a symlink) and chmodded to 0700 so a pre-existing
// permissive directory is tightened.
func (m Manager) Root() (string, error) {
	if m.root == "" {
		return "", ErrInvalidKey
	}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(m.root)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("state root must not be a symlink")
	}
	if !info.IsDir() {
		return "", errors.New("state root is not a directory")
	}
	if err := os.Chmod(m.root, 0o700); err != nil {
		return "", err
	}
	return m.root, nil
}

func (m Manager) path(key, suffix string) (string, error) {
	if err := validKey(key); err != nil {
		return "", err
	}
	root, err := m.Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, key+suffix), nil
}

// Lease is a held OS-level lock. It keeps the *os.File so the descriptor
// stays open until Release; the GC finalizer must not close it early.
type Lease struct {
	path string
	file *os.File
}

// Acquire takes a non-blocking exclusive lock on key+".lock". It returns
// ErrBusy when another execution holds the lock.
func (m Manager) Acquire(key string) (*Lease, error) {
	path, err := m.path(key, ".lock")
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := flock(file.Fd(), lockEx|lockNb); err != nil {
		file.Close()
		if errors.Is(err, errWouldBlock) || errors.Is(err, errAgain) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return &Lease{path: path, file: file}, nil
}

// Release unlocks and closes the same file the lease was acquired on.
// A second Release is a no-op. Unlock and close errors are joined so the
// descriptor is never leaked silently.
func (l *Lease) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	unlockErr := unlock(file.Fd())
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}

// Uncertainty is the durable marker written when an Issue POST may have
// reached GitBucket but the response was lost.
type Uncertainty struct {
	Token       string `json:"token"`
	Phase       string `json:"phase"`
	IssueNumber int    `json:"issue_number,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
}

// ReadUncertainty loads the marker for key. It returns ErrNotFound when
// no marker exists.
func (m Manager) ReadUncertainty(key string) (*Uncertainty, error) {
	path, err := m.path(key, ".uncertain.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var record Uncertainty
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// WriteUncertainty durably writes the marker: a 0600 temp file in the same
// directory, fsynced, closed, renamed into place, then the directory is
// fsynced so the rename itself survives a crash.
func (m Manager) WriteUncertainty(key string, record Uncertainty) error {
	path, err := m.path(key, ".uncertain.json")
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), "uncertain-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// ClearUncertainty removes the marker for key, if present.
func (m Manager) ClearUncertainty(key string) error {
	path, err := m.path(key, ".uncertain.json")
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
