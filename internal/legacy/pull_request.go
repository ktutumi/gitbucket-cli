package legacy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ktutumi/gitbucket-cli/internal/legacyhtml"
)

type CreatePullRequestRequest struct {
	Owner   string
	Repo    string
	Title   string
	Body    string
	Base    string
	Head    string
	BaseSHA string
	HeadSHA string
}

type CreatePullRequestResult struct {
	Number int
	URL    string
}

func (c *Client) CreatePullRequest(ctx context.Context, req CreatePullRequestRequest) (*CreatePullRequestResult, error) {
	if req.Owner == "" || req.Repo == "" {
		return nil, wrap(ErrValidation, "repository must be owner/name")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, wrap(ErrValidation, "--title is required")
	}
	if req.Base == "" || req.Head == "" || req.BaseSHA == "" || req.HeadSHA == "" {
		return nil, wrap(ErrValidation, "--base, --head, --base-sha, and --head-sha are required")
	}
	if !isFullSHA(req.BaseSHA) || !isFullSHA(req.HeadSHA) {
		return nil, wrap(ErrValidation, "--base-sha and --head-sha must be 40-character lowercase hex")
	}

	if err := c.SignIn(ctx); err != nil {
		return nil, err
	}

	resp, err := c.get(ctx, c.repoPath(req.Owner, req.Repo, "compare", req.Base+"..."+req.Head)+"?expand=1")
	if err != nil {
		return nil, wrap(ErrValidation, err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, wrap(ErrValidation, fmt.Sprintf("compare page failed: HTTP %d", resp.StatusCode))
	}
	form, err := legacyhtml.ParseForm(resp.Body, "/pulls/new")
	resp.Body.Close()
	if err != nil {
		return nil, wrap(ErrValidation, fmt.Sprintf("compare page: %v", err))
	}

	targetBranch, err := form.Field("targetBranch")
	if err != nil {
		return nil, wrap(ErrValidation, err.Error())
	}
	requestBranch, err := form.Field("requestBranch")
	if err != nil {
		return nil, wrap(ErrValidation, err.Error())
	}
	commitFrom, err := form.Field("commitIdFrom")
	if err != nil {
		return nil, wrap(ErrValidation, err.Error())
	}
	commitTo, err := form.Field("commitIdTo")
	if err != nil {
		return nil, wrap(ErrValidation, err.Error())
	}
	if targetBranch != req.Base || requestBranch != req.Head || commitFrom != req.BaseSHA || commitTo != req.HeadSHA {
		return nil, wrap(ErrValidation, "compare page identity does not match the caller-fixed base/head")
	}

	values := url.Values{
		"title":            {req.Title},
		"content":          {req.Body},
		"labelNames":       {""},
		"milestoneId":      {""},
		"assignedUserName": {""},
	}
	for _, name := range []string{
		"targetUserName", "targetBranch", "requestUserName",
		"requestRepositoryName", "requestBranch", "commitIdFrom", "commitIdTo",
	} {
		value, err := form.Field(name)
		if err != nil {
			return nil, wrap(ErrValidation, err.Error())
		}
		values.Set(name, value)
	}

	created, err := c.postForm(ctx, c.repoPath(req.Owner, req.Repo, "pulls", "new"), values)
	if err != nil {
		return nil, wrap(ErrLostResponse, err.Error())
	}
	defer created.Body.Close()
	if created.StatusCode != http.StatusFound {
		return nil, wrap(ErrLostResponse, fmt.Sprintf("pull request creation failed: HTTP %d", created.StatusCode))
	}
	number, prURL, err := parseCreatedLocation(c.baseURL, created.Header.Get("Location"), req.Owner, req.Repo, "pull")
	if err != nil {
		return nil, err
	}
	return &CreatePullRequestResult{Number: number, URL: prURL}, nil
}

func isFullSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
