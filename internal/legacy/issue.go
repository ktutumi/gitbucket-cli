package legacy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ktutumi/gitbucket-cli/internal/legacyhtml"
	"github.com/ktutumi/gitbucket-cli/internal/state"
)

const maxIssuePages = 10000

type CreateIssueRequest struct {
	Owner               string
	Repo                string
	Title               string
	Body                string
	MarkerLabel         string
	DedupeKey           string
	OverrideUncertainty bool
}

type CreateIssueResult struct {
	Number int
	URL    string
	Reused bool
}

func CountMarkers(body, label, value string) int {
	want := strings.TrimSpace(label) + ": " + strings.TrimSpace(value)
	count := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == want {
			count++
		}
	}
	return count
}

func (c *Client) CreateIssue(ctx context.Context, mgr state.Manager, req CreateIssueRequest) (*CreateIssueResult, error) {
	if req.Owner == "" || req.Repo == "" {
		return nil, wrap(ErrValidation, "repository must be owner/name")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, wrap(ErrValidation, "--title is required")
	}
	label := strings.TrimSpace(req.MarkerLabel)
	dedupeKey := strings.TrimSpace(req.DedupeKey)
	if label == "" || dedupeKey == "" {
		return nil, wrap(ErrValidation, "--marker-label and --dedupe-key are required")
	}
	if CountMarkers(req.Body, label, dedupeKey) != 1 {
		return nil, wrap(ErrValidation, "issue body must contain exactly one marker line")
	}

	key, err := state.Key(state.Identity{
		URL:         c.baseURL.String(),
		Owner:       req.Owner,
		Repo:        req.Repo,
		MarkerLabel: label,
		DedupeKey:   dedupeKey,
	})
	if err != nil {
		return nil, wrap(ErrValidation, err.Error())
	}
	lease, err := mgr.Acquire(key)
	if err != nil {
		if errors.Is(err, state.ErrBusy) {
			return nil, wrap(ErrBusy, "idempotent create is already running")
		}
		return nil, err
	}
	defer lease.Release()

	if err := c.SignIn(ctx); err != nil {
		return nil, err
	}

	record, recErr := mgr.ReadUncertainty(key)
	hasRecord := recErr == nil
	if recErr != nil && recErr != state.ErrNotFound {
		return nil, recErr
	}

	matches, err := c.scanMatchingIssues(ctx, req.Owner, req.Repo, label, dedupeKey)
	if err != nil {
		if hasRecord && !req.OverrideUncertainty {
			return nil, wrap(ErrUncertainty, "existing uncertainty record; issue scan failed")
		}
		return nil, err
	}
	switch {
	case len(matches) == 1:
		if err := mgr.ClearUncertainty(key); err != nil {
			return nil, err
		}
		return &CreateIssueResult{Number: matches[0].Number, URL: matches[0].URL, Reused: true}, nil
	case len(matches) > 1:
		return nil, wrap(ErrAmbiguous, fmt.Sprintf("multiple issues contain marker %s: %s", dedupeKey, formatNumbers(matches)))
	}

	if hasRecord && !req.OverrideUncertainty {
		_ = record
		return nil, wrap(ErrUncertainty, "uncertainty record exists and no matching issue was found")
	}

	if err := mgr.WriteUncertainty(key, state.Uncertainty{Phase: "posting"}); err != nil {
		return nil, err
	}

	created, err := c.postIssue(ctx, req)
	if err != nil {
		matches, scanErr := c.scanMatchingIssues(ctx, req.Owner, req.Repo, label, dedupeKey)
		if scanErr == nil && len(matches) == 1 {
			if clearErr := mgr.ClearUncertainty(key); clearErr != nil {
				return nil, clearErr
			}
			return &CreateIssueResult{Number: matches[0].Number, URL: matches[0].URL, Reused: true}, nil
		}
		return nil, wrap(ErrUncertainty, "issue creation response was lost; uncertainty record retained")
	}

	if err := mgr.WriteUncertainty(key, state.Uncertainty{Phase: "created", IssueNumber: created.Number}); err != nil {
		return nil, err
	}
	matches, err = c.scanMatchingIssues(ctx, req.Owner, req.Repo, label, dedupeKey)
	if err != nil || len(matches) != 1 || matches[0].Number != created.Number {
		return nil, wrap(ErrUncertainty, "post-create scan did not confirm the created issue; uncertainty record retained")
	}
	if err := mgr.ClearUncertainty(key); err != nil {
		return nil, err
	}
	return created, nil
}

type scannedIssue struct {
	Number int
	URL    string
}

func (c *Client) scanMatchingIssues(ctx context.Context, owner, repo, label, value string) ([]scannedIssue, error) {
	numbers, err := c.listIssueNumbers(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	var matches []scannedIssue
	for _, number := range numbers {
		body, err := c.issueRawMarkdown(ctx, owner, repo, number)
		if err != nil {
			return nil, err
		}
		if CountMarkers(body, label, value) >= 1 {
			matches = append(matches, scannedIssue{Number: number, URL: c.issueURL(owner, repo, number)})
		}
	}
	return matches, nil
}

func (c *Client) listIssueNumbers(ctx context.Context, owner, repo string) ([]int, error) {
	seen := map[int]struct{}{}
	var numbers []int
	for _, issueState := range []string{"open", "closed"} {
		stateIDs := map[int]struct{}{}
		for page := 1; page <= maxIssuePages; page++ {
			query := url.Values{
				"state":     {issueState},
				"sort":      {"created"},
				"direction": {"desc"},
				"page":      {strconv.Itoa(page)},
			}
			resp, err := c.get(ctx, c.repoPath(owner, repo, "issues")+"?"+query.Encode())
			if err != nil {
				return nil, wrap(ErrScanFailed, err.Error())
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, wrap(ErrScanFailed, fmt.Sprintf("%s issue scan page %d failed: HTTP %d", issueState, page, resp.StatusCode))
			}
			body, err := readBody(resp)
			if err != nil {
				return nil, wrap(ErrScanFailed, err.Error())
			}
			links, err := legacyhtml.TableIssueLinks(strings.NewReader(body))
			if err != nil {
				return nil, wrap(ErrScanFailed, fmt.Sprintf("%s issue scan page %d: %v", issueState, page, err))
			}
			if len(links) == 0 {
				break
			}
			var pageIDs []int
			for _, href := range links {
				n, ok := parseIssueNumberFromLink(href, owner, repo)
				if !ok {
					continue
				}
				if _, exists := stateIDs[n]; exists {
					return nil, wrap(ErrScanFailed, fmt.Sprintf("%s issue pagination repeated IDs on page %d", issueState, page))
				}
				stateIDs[n] = struct{}{}
				pageIDs = append(pageIDs, n)
			}
			if len(pageIDs) == 0 {
				return nil, wrap(ErrScanFailed, fmt.Sprintf("%s issue scan page %d has no issue links", issueState, page))
			}
			for _, n := range pageIDs {
				if _, exists := seen[n]; exists {
					continue
				}
				seen[n] = struct{}{}
				numbers = append(numbers, n)
			}
		}
	}
	return numbers, nil
}

func (c *Client) issueRawMarkdown(ctx context.Context, owner, repo string, number int) (string, error) {
	resp, err := c.get(ctx, c.repoPath(owner, repo, "issues", "_data", strconv.Itoa(number))+"?dataType=html")
	if err != nil {
		return "", wrap(ErrScanFailed, err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return "", wrap(ErrScanFailed, fmt.Sprintf("issue #%d raw markdown failed: HTTP %d", number, resp.StatusCode))
	}
	body, err := readBody(resp)
	if err != nil {
		return "", wrap(ErrScanFailed, err.Error())
	}
	content, err := legacyhtml.TextareaByID(strings.NewReader(body), "edit-content")
	if err != nil {
		return "", wrap(ErrScanFailed, fmt.Sprintf("issue #%d raw markdown: %v", number, err))
	}
	return content, nil
}

func (c *Client) postIssue(ctx context.Context, req CreateIssueRequest) (*CreateIssueResult, error) {
	resp, err := c.postForm(ctx, c.repoPath(req.Owner, req.Repo, "issues", "new"), url.Values{
		"title":            {req.Title},
		"content":          {req.Body},
		"labelNames":       {""},
		"milestoneId":      {""},
		"assignedUserName": {""},
	})
	if err != nil {
		return nil, wrap(ErrLostResponse, err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		return nil, wrap(ErrLostResponse, fmt.Sprintf("issue creation failed: HTTP %d", resp.StatusCode))
	}
	number, issueURL, err := parseCreatedLocation(c.baseURL, resp.Header.Get("Location"), req.Owner, req.Repo, "issues")
	if err != nil {
		return nil, err
	}
	return &CreateIssueResult{Number: number, URL: issueURL}, nil
}

func (c *Client) issueURL(owner, repo string, number int) string {
	u := *c.baseURL
	u.Path = strings.TrimRight(c.baseURL.Path, "/") + c.repoPath(owner, repo, "issues", strconv.Itoa(number))
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func formatNumbers(matches []scannedIssue) string {
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		parts = append(parts, fmt.Sprintf("#%d", match.Number))
	}
	return strings.Join(parts, ", ")
}
