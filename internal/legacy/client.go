package legacy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// Client talks to a legacy GitBucket host through Web forms.
// It never follows redirects; each operation validates the 302 Location.
type Client struct {
	baseURL    *url.URL
	user       string
	password   string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

func neverFollowRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func cloneHTTPClient(src *http.Client) *http.Client {
	cloned := &http.Client{}
	if src != nil {
		*cloned = *src
	}
	return cloned
}

func NewClient(baseURL, user, password string, opts ...Option) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, wrap(ErrValidation, "legacy GitBucket URL is required")
	}
	if user == "" {
		return nil, wrap(ErrValidation, "legacy GitBucket sign-in name is required")
	}
	if password == "" {
		return nil, wrap(ErrAuth, "legacy GitBucket password is required; set --password or GITBUCKET_PASSWORD")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, wrap(ErrValidation, "legacy GitBucket URL is invalid")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &Client{
		baseURL:  parsed,
		user:     user,
		password: password,
	}
	for _, opt := range opts {
		opt(client)
	}
	if client.httpClient == nil {
		client.httpClient = &http.Client{}
	} else {
		client.httpClient = cloneHTTPClient(client.httpClient)
	}
	client.httpClient.Jar = jar
	client.httpClient.CheckRedirect = neverFollowRedirects
	return client, nil
}

func (c *Client) SignIn(ctx context.Context) error {
	page, err := c.get(ctx, "/signin")
	if err != nil {
		return wrap(ErrAuth, fmt.Sprintf("signin page failed: %v", err))
	}
	page.Body.Close()
	resp, err := c.postForm(ctx, "/signin", url.Values{
		"userName": {c.user},
		"password": {c.password},
	})
	if err != nil {
		return wrap(ErrAuth, fmt.Sprintf("signin failed: %v", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound || strings.TrimSpace(resp.Header.Get("Location")) == "" {
		return wrap(ErrAuth, fmt.Sprintf("signin failed: HTTP %d", resp.StatusCode))
	}
	location, err := resp.Location()
	if err != nil || strings.TrimRight(location.Path, "/") == strings.TrimRight(c.baseURL.Path, "/")+"/signin" {
		return wrap(ErrAuth, "signin failed: invalid redirect or returned to signin")
	}
	return nil
}

func (c *Client) get(ctx context.Context, requestPath string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(requestPath), nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

func (c *Client) postForm(ctx context.Context, requestPath string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(requestPath), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.httpClient.Do(req)
}

func (c *Client) url(requestPath string) string {
	rel, err := url.Parse(requestPath)
	if err != nil {
		return c.baseURL.String()
	}
	relEscaped := rel.EscapedPath()
	if relEscaped == "" {
		relEscaped = "/"
	}
	if !strings.HasPrefix(relEscaped, "/") {
		relEscaped = "/" + relEscaped
	}
	joinedEscaped := strings.TrimRight(c.baseURL.EscapedPath(), "/") + relEscaped
	decoded, err := url.PathUnescape(joinedEscaped)
	if err != nil {
		decoded = joinedEscaped
	}
	joined := *c.baseURL
	joined.Path = decoded
	joined.RawPath = joinedEscaped
	joined.RawQuery = rel.RawQuery
	joined.Fragment = ""
	return joined.String()
}

func (c *Client) repoPath(owner, repo string, parts ...string) string {
	all := append([]string{owner, repo}, parts...)
	escaped := make([]string, 0, len(all))
	for _, part := range all {
		escaped = append(escaped, url.PathEscape(part))
	}
	return "/" + strings.Join(escaped, "/")
}

func parseCreatedLocation(base *url.URL, location, owner, repo, kind string) (int, string, error) {
	if strings.TrimSpace(location) == "" {
		return 0, "", wrap(ErrLostResponse, kind+" Location is missing")
	}
	ref, err := url.Parse(location)
	if err != nil {
		return 0, "", wrap(ErrLostResponse, kind+" Location is invalid")
	}
	resolved := base.ResolveReference(ref)
	wantPrefix := path.Join("/", strings.Trim(base.Path, "/"), owner, repo, kind) + "/"
	gotPath := resolved.EscapedPath()
	if !strings.HasPrefix(gotPath, wantPrefix) {
		return 0, "", wrap(ErrLostResponse, fmt.Sprintf("unexpected %s Location: %s", kind, resolved))
	}
	rest := strings.Trim(strings.TrimPrefix(gotPath, wantPrefix), "/")
	if rest == "" || strings.Contains(rest, "/") {
		return 0, "", wrap(ErrLostResponse, fmt.Sprintf("unexpected %s Location: %s", kind, resolved))
	}
	number, err := strconv.Atoi(rest)
	if err != nil || number <= 0 {
		return 0, "", wrap(ErrLostResponse, fmt.Sprintf("unexpected %s Location: %s", kind, resolved))
	}
	canonical := *resolved
	canonical.RawQuery = ""
	canonical.Fragment = ""
	canonical.Path = path.Join("/", strings.Trim(base.Path, "/"), owner, repo, kind, strconv.Itoa(number))
	return number, canonical.String(), nil
}

func readBody(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parseIssueNumberFromLink(href, owner, repo string) (int, bool) {
	parsed, err := url.Parse(href)
	if err != nil {
		return 0, false
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) < 4 {
		return 0, false
	}
	// Paths may be prefixed by a context root. Match the last owner/repo/issues/N.
	for i := 0; i <= len(parts)-4; i++ {
		if parts[i] == url.PathEscape(owner) && parts[i+1] == url.PathEscape(repo) && parts[i+2] == "issues" {
			n, err := strconv.Atoi(parts[i+3])
			if err == nil && n > 0 && i+4 == len(parts) {
				return n, true
			}
		}
	}
	return 0, false
}
