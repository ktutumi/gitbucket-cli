package gitbucket

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Client struct {
	baseURL    *url.URL
	token      string
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

func NewClient(baseURL, token string, opts ...Option) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if !strings.HasSuffix(baseURL, "/api/v3") {
		baseURL += "/api/v3"
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	client := &Client{
		baseURL:    parsed,
		token:      token,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

type Repo struct {
	Owner string
	Name  string
}

type User struct {
	Login     string `json:"login"`
	Email     string `json:"email,omitempty"`
	Type      string `json:"type,omitempty"`
	SiteAdmin bool   `json:"site_admin,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
}

type Signature struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Date  string `json:"date,omitempty"`
}

type CommitDetails struct {
	Message   string    `json:"message"`
	Author    Signature `json:"author"`
	Committer Signature `json:"committer"`
}

type Commit struct {
	SHA     string        `json:"sha"`
	HTMLURL string        `json:"html_url,omitempty"`
	Commit  CommitDetails `json:"commit"`
	Author  *User         `json:"author,omitempty"`
}

type ListCommitsOptions struct {
	Branch string
	Author string
	Limit  int
}

type Content struct {
	Path    string `json:"path"`
	SHA     string `json:"sha"`
	Content string `json:"content,omitempty"`
}

type PutContentRequest struct {
	Message string
	Content []byte
	Branch  string
	SHA     string
}

type PutContentResponse struct {
	Content Content `json:"content"`
	Commit  Commit  `json:"commit"`
}

type Repository struct {
	Name     string `json:"name,omitempty"`
	FullName string `json:"full_name,omitempty"`
	HTMLURL  string `json:"html_url,omitempty"`
	CloneURL string `json:"clone_url,omitempty"`
	SSHURL   string `json:"ssh_url,omitempty"`
}

type PullRequestRef struct {
	Ref  string      `json:"ref"`
	SHA  string      `json:"sha,omitempty"`
	Repo *Repository `json:"repo,omitempty"`
}

type PullRequest struct {
	Number  int            `json:"number"`
	State   string         `json:"state"`
	Title   string         `json:"title"`
	Body    string         `json:"body,omitempty"`
	HTMLURL string         `json:"html_url,omitempty"`
	User    *User          `json:"user,omitempty"`
	Head    PullRequestRef `json:"head"`
	Base    PullRequestRef `json:"base"`
	Merged  bool           `json:"merged,omitempty"`
}

type CreatePullRequestRequest struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Head  string `json:"head"`
	Base  string `json:"base"`
}

type ListPullRequestsOptions struct {
	State    string
	Author   string
	Assignee string
	Limit    int
}

type MergePullRequestRequest struct {
	Method        string
	CommitTitle   string
	CommitMessage string
}

type MergePullRequestResponse struct {
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
	SHA     string `json:"sha,omitempty"`
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("GitBucket API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("GitBucket API returned HTTP %d: %s", e.StatusCode, e.Message)
}

func (c *Client) CurrentUser(ctx context.Context) (*User, error) {
	var user User
	if err := c.do(ctx, http.MethodGet, "/user", nil, nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) ListCommits(ctx context.Context, repo Repo, opts ListCommitsOptions) ([]Commit, error) {
	query := url.Values{}
	if opts.Branch != "" {
		query.Set("sha", opts.Branch)
	}
	if opts.Author != "" {
		query.Set("author", opts.Author)
	}
	if opts.Limit > 0 {
		query.Set("per_page", strconv.Itoa(opts.Limit))
	}

	var commits []Commit
	err := c.do(ctx, http.MethodGet, repoPath(repo, "commits"), query, nil, &commits)
	return commits, err
}

func (c *Client) GetCommit(ctx context.Context, repo Repo, ref string) (*Commit, error) {
	var commit Commit
	err := c.do(ctx, http.MethodGet, repoPath(repo, "commits", ref), nil, nil, &commit)
	if err != nil {
		return nil, err
	}
	return &commit, nil
}

func (c *Client) GetContent(ctx context.Context, repo Repo, contentPath string, branch string) (*Content, error) {
	query := url.Values{}
	if branch != "" {
		query.Set("ref", branch)
	}
	var content Content
	err := c.do(ctx, http.MethodGet, repoContentPath(repo, contentPath), query, nil, &content)
	if err != nil {
		return nil, err
	}
	return &content, nil
}

func (c *Client) PutContent(ctx context.Context, repo Repo, contentPath string, req PutContentRequest) (*PutContentResponse, error) {
	payload := map[string]string{
		"message": req.Message,
		"content": base64.StdEncoding.EncodeToString(req.Content),
	}
	if req.Branch != "" {
		payload["branch"] = req.Branch
	}
	if req.SHA != "" {
		payload["sha"] = req.SHA
	}

	var response PutContentResponse
	err := c.do(ctx, http.MethodPut, repoContentPath(repo, contentPath), nil, payload, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) CreatePullRequest(ctx context.Context, repo Repo, req CreatePullRequestRequest) (*PullRequest, error) {
	var pr PullRequest
	err := c.do(ctx, http.MethodPost, repoPath(repo, "pulls"), nil, req, &pr)
	if err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) ListPullRequests(ctx context.Context, repo Repo, opts ListPullRequestsOptions) ([]PullRequest, error) {
	query := url.Values{}
	if opts.State != "" {
		query.Set("state", opts.State)
	}
	if opts.Author != "" {
		query.Set("author", opts.Author)
	}
	if opts.Assignee != "" {
		query.Set("assignee", opts.Assignee)
	}
	if opts.Limit > 0 {
		query.Set("per_page", strconv.Itoa(opts.Limit))
	}

	var prs []PullRequest
	err := c.do(ctx, http.MethodGet, repoPath(repo, "pulls"), query, nil, &prs)
	return prs, err
}

func (c *Client) GetPullRequest(ctx context.Context, repo Repo, number int) (*PullRequest, error) {
	var pr PullRequest
	err := c.do(ctx, http.MethodGet, repoPath(repo, "pulls", strconv.Itoa(number)), nil, nil, &pr)
	if err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) MergePullRequest(ctx context.Context, repo Repo, number int, req MergePullRequestRequest) (*MergePullRequestResponse, error) {
	payload := map[string]string{}
	if req.Method != "" {
		payload["merge_method"] = req.Method
	}
	if req.CommitTitle != "" {
		payload["commit_title"] = req.CommitTitle
	}
	if req.CommitMessage != "" {
		payload["commit_message"] = req.CommitMessage
	}

	var response MergePullRequestResponse
	err := c.do(ctx, http.MethodPut, repoPath(repo, "pulls", strconv.Itoa(number), "merge"), nil, payload, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) DeleteBranch(ctx context.Context, repo Repo, branch string) error {
	return c.do(ctx, http.MethodDelete, repoRefPath(repo, "heads/"+branch), nil, nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, payload any, out any) error {
	req, err := c.newRequest(ctx, method, path, query, payload)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) newRequest(ctx context.Context, method, requestPath string, query url.Values, payload any) (*http.Request, error) {
	u := *c.baseURL
	u.Path = strings.TrimRight(c.baseURL.Path, "/") + requestPath
	u.RawQuery = query.Encode()

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	return req, nil
}

func decodeAPIError(resp *http.Response) error {
	data, _ := io.ReadAll(resp.Body)
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &payload)
	if payload.Message == "" {
		payload.Message = strings.TrimSpace(string(data))
	}
	return &APIError{StatusCode: resp.StatusCode, Message: payload.Message}
}

func repoPath(repo Repo, parts ...string) string {
	all := []string{"repos", repo.Owner, repo.Name}
	all = append(all, parts...)
	return "/" + escapeSegments(all...)
}

func repoContentPath(repo Repo, contentPath string) string {
	parts := []string{"repos", repo.Owner, repo.Name, "contents"}
	parts = append(parts, splitPath(contentPath)...)
	return "/" + escapeSegments(parts...)
}

func repoRefPath(repo Repo, ref string) string {
	parts := []string{"repos", repo.Owner, repo.Name, "git", "refs"}
	parts = append(parts, splitPath(ref)...)
	return "/" + escapeSegments(parts...)
}

func splitPath(value string) []string {
	var parts []string
	for _, part := range strings.Split(value, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func escapeSegments(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return strings.Join(escaped, "/")
}
