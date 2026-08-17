package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/ktutumi/gitbucket-cli/internal/config"
	"github.com/ktutumi/gitbucket-cli/internal/gitbucket"
	"github.com/ktutumi/gitbucket-cli/internal/legacy"
	"github.com/ktutumi/gitbucket-cli/internal/state"
)

const version = "0.1.0"

type Options struct {
	Stdout     io.Writer
	Stderr     io.Writer
	ConfigPath string
	StateDir   string
	Env        map[string]string
	WorkDir    string

	HTTPClient  *http.Client
	RunCommand  func(context.Context, string, ...string) error
	OpenBrowser func(string) error
}

type globals struct {
	repo string
	url  string
	json bool
}

func Run(ctx context.Context, args []string, opts Options) int {
	runner := newRunner(opts)
	if err := runner.run(ctx, args); err != nil {
		fmt.Fprintln(runner.stderr, "bkt:", err)
		return exitCode(err)
	}
	return 0
}

func exitCode(err error) int {
	switch {
	case errors.Is(err, legacy.ErrUncertainty):
		return 2
	case errors.Is(err, legacy.ErrAmbiguous):
		return 3
	case errors.Is(err, legacy.ErrAuth):
		return 4
	case errors.Is(err, legacy.ErrValidation):
		return 5
	default:
		return 1
	}
}

type runner struct {
	stdout   io.Writer
	stderr   io.Writer
	store    config.Store
	stateDir string
	env      map[string]string
	workDir  string

	httpClient  *http.Client
	runCommand  func(context.Context, string, ...string) error
	openBrowser func(string) error
}

func newRunner(opts Options) *runner {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	env := map[string]string{}
	for _, key := range []string{"GITBUCKET_URL", "GITBUCKET_TOKEN", "GITBUCKET_REPO", "GITBUCKET_PASSWORD"} {
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}
	for key, value := range opts.Env {
		env[key] = value
	}
	runCommand := opts.RunCommand
	if runCommand == nil {
		runCommand = defaultRunCommand
	}
	openBrowser := opts.OpenBrowser
	if openBrowser == nil {
		openBrowser = defaultOpenBrowser
	}
	return &runner{
		stdout:      stdout,
		stderr:      stderr,
		store:       config.NewStore(opts.ConfigPath),
		stateDir:    opts.StateDir,
		env:         env,
		workDir:     opts.WorkDir,
		httpClient:  opts.HTTPClient,
		runCommand:  runCommand,
		openBrowser: openBrowser,
	}
}

func (r *runner) run(ctx context.Context, args []string) error {
	globals, rest, err := parseLeadingGlobals(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		printRootHelp(r.stdout)
		return nil
	}

	switch rest[0] {
	case "help", "--help", "-h":
		printRootHelp(r.stdout)
		return nil
	case "--version", "-v", "version":
		fmt.Fprintln(r.stdout, version)
		return nil
	case "auth":
		return r.runAuth(ctx, rest[1:], globals)
	case "commit":
		return r.runCommit(ctx, rest[1:], globals)
	case "issue":
		return r.runIssue(ctx, rest[1:], globals)
	case "pr":
		return r.runPR(ctx, rest[1:], globals)
	default:
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func parseLeadingGlobals(args []string) (globals, []string, error) {
	var g globals
	for len(args) > 0 {
		switch args[0] {
		case "--repo", "-R":
			if len(args) < 2 {
				return g, nil, fmt.Errorf("%s requires a value", args[0])
			}
			g.repo = args[1]
			args = args[2:]
		case "--url", "-u":
			if len(args) < 2 {
				return g, nil, fmt.Errorf("%s requires a value", args[0])
			}
			g.url = args[1]
			args = args[2:]
		case "--json":
			g.json = true
			args = args[1:]
		default:
			return g, args, nil
		}
	}
	return g, args, nil
}

func (r *runner) runAuth(ctx context.Context, args []string, g globals) error {
	if len(args) == 0 {
		return errors.New("auth subcommand is required")
	}
	switch args[0] {
	case "login":
		fs := newFlagSet("auth login", r.stderr)
		token := fs.String("token", "", "personal access token")
		fs.StringVar(token, "t", "", "personal access token")
		urlValue := fs.String("url", g.url, "GitBucket URL")
		fs.StringVar(urlValue, "u", g.url, "GitBucket URL")
		legacyMode := fs.Bool("legacy", false, "declare a legacy GitBucket host")
		user := fs.String("user", "", "legacy sign-in name")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		url := config.NormalizeURL(*urlValue)
		if url == "" {
			return errors.New("--url is required")
		}
		cfg, err := r.store.Load()
		if err != nil {
			return err
		}
		if *legacyMode {
			if strings.TrimSpace(*token) != "" {
				return wrapValidation("legacy login does not accept --token")
			}
			if strings.TrimSpace(*user) == "" {
				return wrapValidation("--user is required for legacy login")
			}
			cfg = config.UpsertHost(cfg, url, config.HostConfig{Legacy: true, User: strings.TrimSpace(*user)})
			if err := r.store.Save(cfg); err != nil {
				return err
			}
			fmt.Fprintf(r.stdout, "Logged in to %s\n", url)
			return nil
		}
		tokenValue := *token
		if tokenValue == "" {
			tokenValue = r.env["GITBUCKET_TOKEN"]
		}
		if tokenValue == "" {
			return errors.New("--token or GITBUCKET_TOKEN is required")
		}
		cfg = config.UpsertHost(cfg, url, config.HostConfig{Token: tokenValue})
		if err := r.store.Save(cfg); err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "Logged in to %s\n", url)
		return nil
	case "status":
		fs := newFlagSet("auth status", r.stderr)
		urlValue := fs.String("url", g.url, "GitBucket URL")
		fs.StringVar(urlValue, "u", g.url, "GitBucket URL")
		jsonOut := fs.Bool("json", g.json, "output JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		resolved, err := r.resolveHost(*urlValue)
		if err != nil {
			return err
		}
		if resolved.Legacy {
			if *jsonOut {
				return writeJSON(r.stdout, map[string]any{
					"url":    resolved.URL,
					"legacy": true,
					"user":   resolved.User,
				})
			}
			fmt.Fprintf(r.stdout, "Logged in to %s as %s (legacy; verified on command)\n", resolved.URL, resolved.User)
			return nil
		}
		client, resolved, err := r.client(ctx, *urlValue)
		if err != nil {
			return err
		}
		user, err := client.CurrentUser(ctx)
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(r.stdout, map[string]any{"url": resolved.URL, "user": user})
		}
		fmt.Fprintf(r.stdout, "Logged in to %s as %s\n", resolved.URL, user.Login)
		return nil
	case "logout":
		fs := newFlagSet("auth logout", r.stderr)
		urlValue := fs.String("url", g.url, "GitBucket URL")
		fs.StringVar(urlValue, "u", g.url, "GitBucket URL")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		cfg, err := r.store.Load()
		if err != nil {
			return err
		}
		url := config.NormalizeURL(*urlValue)
		if url == "" {
			url = config.NormalizeURL(cfg.DefaultURL)
		}
		if url == "" {
			return errors.New("no GitBucket host is configured")
		}
		cfg = config.RemoveHost(cfg, url)
		if err := r.store.Save(cfg); err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "Logged out from %s\n", url)
		return nil
	default:
		return fmt.Errorf("unknown auth subcommand %q", args[0])
	}
}

func (r *runner) runCommit(ctx context.Context, args []string, g globals) error {
	if len(args) == 0 {
		return errors.New("commit subcommand is required")
	}
	switch args[0] {
	case "list":
		fs := newFlagSet("commit list", r.stderr)
		repoValue := fs.String("repo", g.repo, "repository owner/name")
		fs.StringVar(repoValue, "R", g.repo, "repository owner/name")
		branch := fs.String("branch", "", "branch")
		fs.StringVar(branch, "b", "", "branch")
		author := fs.String("author", "", "author")
		fs.StringVar(author, "a", "", "author")
		limit := fs.Int("limit", 20, "limit")
		fs.IntVar(limit, "L", 20, "limit")
		jsonOut := fs.Bool("json", g.json, "output JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		client, repo, err := r.clientAndRepo(ctx, g.url, *repoValue)
		if err != nil {
			return err
		}
		commits, err := client.ListCommits(ctx, repo, gitbucket.ListCommitsOptions{Branch: *branch, Author: *author, Limit: *limit})
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(r.stdout, commits)
		}
		writeCommitList(r.stdout, commits)
		return nil
	case "view":
		fs := newFlagSet("commit view", r.stderr)
		repoValue := fs.String("repo", g.repo, "repository owner/name")
		fs.StringVar(repoValue, "R", g.repo, "repository owner/name")
		jsonOut := fs.Bool("json", g.json, "output JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("commit SHA is required")
		}
		client, repo, err := r.clientAndRepo(ctx, g.url, *repoValue)
		if err != nil {
			return err
		}
		commit, err := client.GetCommit(ctx, repo, fs.Arg(0))
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(r.stdout, commit)
		}
		writeCommitView(r.stdout, commit)
		return nil
	case "create":
		fs := newFlagSet("commit create", r.stderr)
		repoValue := fs.String("repo", g.repo, "repository owner/name")
		fs.StringVar(repoValue, "R", g.repo, "repository owner/name")
		message := fs.String("message", "", "commit message")
		fs.StringVar(message, "m", "", "commit message")
		messageFile := fs.String("file", "", "commit message file")
		fs.StringVar(messageFile, "F", "", "commit message file")
		contentPath := fs.String("path", "", "repository path")
		fs.StringVar(contentPath, "p", "", "repository path")
		content := fs.String("content", "", "file content")
		fs.StringVar(content, "c", "", "file content")
		contentFile := fs.String("content-file", "", "file content path")
		branch := fs.String("branch", "", "branch")
		fs.StringVar(branch, "b", "", "branch")
		sha := fs.String("sha", "", "existing blob sha")
		jsonOut := fs.Bool("json", g.json, "output JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		msg, err := readMessage(*message, *messageFile)
		if err != nil {
			return err
		}
		if msg == "" {
			return errors.New("--message or --file is required")
		}
		if *contentPath == "" {
			return errors.New("--path is required")
		}
		body, err := readContent(*content, *contentFile)
		if err != nil {
			return err
		}
		client, repo, err := r.clientAndRepo(ctx, g.url, *repoValue)
		if err != nil {
			return err
		}
		blobSHA := *sha
		if blobSHA == "" {
			blobSHA, err = r.existingContentSHA(ctx, client, repo, *contentPath, *branch)
			if err != nil {
				return err
			}
		}
		result, err := client.PutContent(ctx, repo, *contentPath, gitbucket.PutContentRequest{
			Message: msg,
			Content: body,
			Branch:  *branch,
			SHA:     blobSHA,
		})
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(r.stdout, result)
		}
		fmt.Fprintf(r.stdout, "Created commit %s\n", result.Commit.SHA)
		if result.Commit.HTMLURL != "" {
			fmt.Fprintln(r.stdout, result.Commit.HTMLURL)
		}
		return nil
	default:
		return fmt.Errorf("unknown commit subcommand %q", args[0])
	}
}

func (r *runner) runIssue(ctx context.Context, args []string, g globals) error {
	if len(args) == 0 {
		return errors.New("issue subcommand is required")
	}
	switch args[0] {
	case "create":
		fs := newFlagSet("issue create", r.stderr)
		repoValue := fs.String("repo", g.repo, "repository owner/name")
		fs.StringVar(repoValue, "R", g.repo, "repository owner/name")
		title := fs.String("title", "", "title")
		fs.StringVar(title, "t", "", "title")
		body := fs.String("body", "", "body")
		fs.StringVar(body, "b", "", "body")
		bodyFile := fs.String("body-file", "", "body file")
		fs.StringVar(bodyFile, "F", "", "body file")
		markerLabel := fs.String("marker-label", "", "idempotent marker label")
		dedupeKey := fs.String("dedupe-key", "", "idempotent marker value")
		override := fs.Bool("override-uncertainty", false, "allow create when an uncertainty record exists")
		password := fs.String("password", "", "legacy GitBucket password")
		jsonOut := fs.Bool("json", g.json, "output JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		resolved, err := r.resolveHost(g.url)
		if err != nil {
			return err
		}
		if !resolved.Legacy {
			return wrapUnsupported("issue create is only supported on a legacy GitBucket host")
		}
		bodyText, err := readMessage(*body, *bodyFile)
		if err != nil {
			return err
		}
		repo, err := r.resolveRepo(ctx, *repoValue)
		if err != nil {
			return err
		}
		client, err := r.legacyClient(resolved, r.password(*password))
		if err != nil {
			return err
		}
		result, err := client.CreateIssue(ctx, state.NewManager(r.stateDir), legacy.CreateIssueRequest{
			Owner:               repo.Owner,
			Repo:                repo.Name,
			Title:               *title,
			Body:                bodyText,
			MarkerLabel:         *markerLabel,
			DedupeKey:           *dedupeKey,
			OverrideUncertainty: *override,
		})
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(r.stdout, result)
		}
		if result.Reused {
			fmt.Fprintf(r.stdout, "Reused issue #%d\n", result.Number)
		} else {
			fmt.Fprintf(r.stdout, "Created issue #%d\n", result.Number)
		}
		if result.URL != "" {
			fmt.Fprintln(r.stdout, result.URL)
		}
		return nil
	default:
		return fmt.Errorf("unknown issue subcommand %q", args[0])
	}
}

func (r *runner) runPR(ctx context.Context, args []string, g globals) error {
	if len(args) == 0 {
		return errors.New("pr subcommand is required")
	}
	switch args[0] {
	case "create":
		fs := newFlagSet("pr create", r.stderr)
		repoValue := fs.String("repo", g.repo, "repository owner/name")
		fs.StringVar(repoValue, "R", g.repo, "repository owner/name")
		title := fs.String("title", "", "title")
		fs.StringVar(title, "t", "", "title")
		body := fs.String("body", "", "body")
		fs.StringVar(body, "b", "", "body")
		bodyFile := fs.String("body-file", "", "body file")
		fs.StringVar(bodyFile, "F", "", "body file")
		base := fs.String("base", "main", "base branch")
		fs.StringVar(base, "B", "main", "base branch")
		head := fs.String("head", "", "head branch")
		fs.StringVar(head, "H", "", "head branch")
		baseSHA := fs.String("base-sha", "", "caller-fixed base commit SHA")
		headSHA := fs.String("head-sha", "", "caller-fixed head commit SHA")
		password := fs.String("password", "", "legacy GitBucket password")
		web := fs.Bool("web", false, "open in browser")
		fs.BoolVar(web, "w", false, "open in browser")
		jsonOut := fs.Bool("json", g.json, "output JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		resolved, err := r.resolveHost(g.url)
		if err != nil {
			return err
		}
		if strings.TrimSpace(*title) == "" {
			if resolved.Legacy {
				return wrapValidation("--title is required")
			}
			return errors.New("--title is required")
		}
		bodyText, err := readMessage(*body, *bodyFile)
		if err != nil {
			return err
		}
		if resolved.Legacy {
			supplied := visitedFlags(fs)
			if !flagSetHas(supplied, "base", "B") || !flagSetHas(supplied, "head", "H") || !flagSetHas(supplied, "base-sha") || !flagSetHas(supplied, "head-sha") {
				return wrapValidation("--base, --head, --base-sha, and --head-sha are required")
			}
			repo, err := r.resolveRepo(ctx, *repoValue)
			if err != nil {
				return err
			}
			client, err := r.legacyClient(resolved, r.password(*password))
			if err != nil {
				return err
			}
			pr, err := client.CreatePullRequest(ctx, legacy.CreatePullRequestRequest{
				Owner:   repo.Owner,
				Repo:    repo.Name,
				Title:   *title,
				Body:    bodyText,
				Base:    *base,
				Head:    *head,
				BaseSHA: *baseSHA,
				HeadSHA: *headSHA,
			})
			if err != nil {
				return err
			}
			if *jsonOut {
				return writeJSON(r.stdout, pr)
			}
			fmt.Fprintf(r.stdout, "Created pull request #%d\n", pr.Number)
			if pr.URL != "" {
				fmt.Fprintln(r.stdout, pr.URL)
			}
			return nil
		}
		if *baseSHA != "" || *headSHA != "" {
			return errors.New("--base-sha and --head-sha are only valid on a legacy GitBucket host")
		}
		if *head == "" {
			current, err := r.currentBranch(ctx)
			if err != nil {
				return errors.New("--head is required when current git branch cannot be detected")
			}
			*head = current
		}
		client, repo, err := r.clientAndRepo(ctx, g.url, *repoValue)
		if err != nil {
			return err
		}
		pr, err := client.CreatePullRequest(ctx, repo, gitbucket.CreatePullRequestRequest{
			Title: *title,
			Body:  bodyText,
			Head:  *head,
			Base:  *base,
		})
		if err != nil {
			return err
		}
		if *web && pr.HTMLURL != "" {
			_ = r.openBrowser(pr.HTMLURL)
		}
		if *jsonOut {
			return writeJSON(r.stdout, pr)
		}
		fmt.Fprintf(r.stdout, "Created pull request #%d\n", pr.Number)
		if pr.HTMLURL != "" {
			fmt.Fprintln(r.stdout, pr.HTMLURL)
		}
		return nil
	case "list":
		fs := newFlagSet("pr list", r.stderr)
		repoValue := fs.String("repo", g.repo, "repository owner/name")
		fs.StringVar(repoValue, "R", g.repo, "repository owner/name")
		state := fs.String("state", "open", "state")
		fs.StringVar(state, "s", "open", "state")
		author := fs.String("author", "", "author")
		fs.StringVar(author, "a", "", "author")
		assignee := fs.String("assignee", "", "assignee")
		fs.StringVar(assignee, "A", "", "assignee")
		limit := fs.Int("limit", 20, "limit")
		fs.IntVar(limit, "L", 20, "limit")
		jsonOut := fs.Bool("json", g.json, "output JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		client, repo, err := r.clientAndRepo(ctx, g.url, *repoValue)
		if err != nil {
			return err
		}
		prs, err := client.ListPullRequests(ctx, repo, gitbucket.ListPullRequestsOptions{
			State:    *state,
			Author:   *author,
			Assignee: *assignee,
			Limit:    *limit,
		})
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(r.stdout, prs)
		}
		writePRList(r.stdout, prs)
		return nil
	case "view":
		fs := newFlagSet("pr view", r.stderr)
		repoValue := fs.String("repo", g.repo, "repository owner/name")
		fs.StringVar(repoValue, "R", g.repo, "repository owner/name")
		web := fs.Bool("web", false, "open in browser")
		fs.BoolVar(web, "w", false, "open in browser")
		jsonOut := fs.Bool("json", g.json, "output JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		number, err := oneNumberArg(fs, "pull request number is required")
		if err != nil {
			return err
		}
		client, repo, err := r.clientAndRepo(ctx, g.url, *repoValue)
		if err != nil {
			return err
		}
		pr, err := client.GetPullRequest(ctx, repo, number)
		if err != nil {
			return err
		}
		if *web && pr.HTMLURL != "" {
			_ = r.openBrowser(pr.HTMLURL)
		}
		if *jsonOut {
			return writeJSON(r.stdout, pr)
		}
		writePRView(r.stdout, pr)
		return nil
	case "merge":
		fs := newFlagSet("pr merge", r.stderr)
		repoValue := fs.String("repo", g.repo, "repository owner/name")
		fs.StringVar(repoValue, "R", g.repo, "repository owner/name")
		merge := fs.Bool("merge", false, "merge commit")
		squash := fs.Bool("squash", false, "squash merge")
		rebase := fs.Bool("rebase", false, "rebase merge")
		deleteBranch := fs.Bool("delete-branch", false, "delete branch")
		jsonOut := fs.Bool("json", g.json, "output JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		number, err := oneNumberArg(fs, "pull request number is required")
		if err != nil {
			return err
		}
		method, err := mergeMethod(*merge, *squash, *rebase)
		if err != nil {
			return err
		}
		client, repo, err := r.clientAndRepo(ctx, g.url, *repoValue)
		if err != nil {
			return err
		}
		var pr *gitbucket.PullRequest
		if *deleteBranch {
			pr, err = client.GetPullRequest(ctx, repo, number)
			if err != nil {
				return err
			}
		}
		result, err := client.MergePullRequest(ctx, repo, number, gitbucket.MergePullRequestRequest{Method: method})
		if err != nil {
			return err
		}
		if *deleteBranch && pr != nil && pr.Head.Ref != "" {
			if err := client.DeleteBranch(ctx, repo, pr.Head.Ref); err != nil {
				return err
			}
		}
		if *jsonOut {
			return writeJSON(r.stdout, result)
		}
		if result.Merged {
			fmt.Fprintf(r.stdout, "Merged pull request #%d\n", number)
		} else {
			fmt.Fprintf(r.stdout, "%s\n", result.Message)
		}
		return nil
	case "checkout":
		fs := newFlagSet("pr checkout", r.stderr)
		repoValue := fs.String("repo", g.repo, "repository owner/name")
		fs.StringVar(repoValue, "R", g.repo, "repository owner/name")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		number, err := oneNumberArg(fs, "pull request number is required")
		if err != nil {
			return err
		}
		client, repo, err := r.clientAndRepo(ctx, g.url, *repoValue)
		if err != nil {
			return err
		}
		pr, err := client.GetPullRequest(ctx, repo, number)
		if err != nil {
			return err
		}
		if pr.Head.Ref == "" {
			return errors.New("pull request head branch is empty")
		}
		if err := r.runCommand(ctx, "git", "fetch", "origin", pr.Head.Ref); err != nil {
			return err
		}
		if err := r.runCommand(ctx, "git", "checkout", pr.Head.Ref); err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "Checked out %s\n", pr.Head.Ref)
		return nil
	default:
		return fmt.Errorf("unknown pr subcommand %q", args[0])
	}
}

func (r *runner) resolveHost(explicitURL string) (config.Resolved, error) {
	cfg, err := r.store.Load()
	if err != nil {
		return config.Resolved{}, err
	}
	return config.Resolve(cfg, r.env, explicitURL)
}

func (r *runner) client(ctx context.Context, explicitURL string) (*gitbucket.Client, config.Resolved, error) {
	resolved, err := r.resolveHost(explicitURL)
	if err != nil {
		return nil, config.Resolved{}, err
	}
	if resolved.Legacy {
		return nil, config.Resolved{}, wrapUnsupported("command is not supported on a legacy GitBucket host")
	}
	opts := []gitbucket.Option{}
	if r.httpClient != nil {
		opts = append(opts, gitbucket.WithHTTPClient(r.httpClient))
	}
	client, err := gitbucket.NewClient(resolved.URL, resolved.Token, opts...)
	if err != nil {
		return nil, config.Resolved{}, err
	}
	_ = ctx
	return client, resolved, nil
}

func (r *runner) clientAndRepo(ctx context.Context, explicitURL, explicitRepo string) (*gitbucket.Client, gitbucket.Repo, error) {
	client, _, err := r.client(ctx, explicitURL)
	if err != nil {
		return nil, gitbucket.Repo{}, err
	}
	repo, err := r.resolveRepo(ctx, explicitRepo)
	if err != nil {
		return nil, gitbucket.Repo{}, err
	}
	return client, repo, nil
}

func (r *runner) legacyClient(resolved config.Resolved, password string) (*legacy.Client, error) {
	opts := []legacy.Option{}
	if r.httpClient != nil {
		opts = append(opts, legacy.WithHTTPClient(r.httpClient))
	}
	return legacy.NewClient(resolved.URL, resolved.User, password, opts...)
}

func (r *runner) password(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return r.env["GITBUCKET_PASSWORD"]
}

func (r *runner) resolveRepo(ctx context.Context, explicitRepo string) (gitbucket.Repo, error) {
	value := explicitRepo
	if value == "" {
		value = r.env["GITBUCKET_REPO"]
	}
	if value == "" {
		remote, err := r.originRemote(ctx)
		if err != nil {
			return gitbucket.Repo{}, errors.New("repository is required; pass --repo owner/name or set GITBUCKET_REPO")
		}
		value = remote
	}
	return parseRepo(value)
}

func (r *runner) originRemote(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	if r.workDir != "" {
		cmd.Dir = r.workDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	remote := strings.TrimSpace(string(out))
	if remote == "" {
		return "", errors.New("origin remote is empty")
	}
	return remote, nil
}

func parseRepo(value string) (gitbucket.Repo, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return gitbucket.Repo{}, fmt.Errorf("repository must be owner/name, got %q", value)
	}
	if !strings.Contains(value, "://") && !strings.Contains(value, "@") {
		value = strings.Trim(value, "/")
		parts := strings.Split(value, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return gitbucket.Repo{}, fmt.Errorf("repository must be owner/name, got %q", value)
		}
		return gitbucket.Repo{Owner: parts[0], Name: strings.TrimSuffix(parts[1], ".git")}, nil
	}

	path := remotePath(value)
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[len(parts)-2] == "" || parts[len(parts)-1] == "" {
		return gitbucket.Repo{}, fmt.Errorf("repository must be owner/name, got %q", value)
	}
	return gitbucket.Repo{Owner: parts[len(parts)-2], Name: parts[len(parts)-1]}, nil
}

func remotePath(value string) string {
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return value
		}
		return parsed.Path
	}
	_, rest, ok := strings.Cut(value, ":")
	if !ok {
		return value
	}
	return rest
}

func (r *runner) existingContentSHA(ctx context.Context, client *gitbucket.Client, repo gitbucket.Repo, path, branch string) (string, error) {
	content, err := client.GetContent(ctx, repo, path, branch)
	if err == nil {
		return content.SHA, nil
	}
	var apiErr *gitbucket.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return "", nil
	}
	return "", err
}

func (r *runner) currentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if r.workDir != "" {
		cmd.Dir = r.workDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return "", errors.New("current branch is not available")
	}
	return branch, nil
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	return fs.Parse(interspersedFlagsFirst(fs, args))
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	seen := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		seen[f.Name] = true
	})
	return seen
}

func flagSetHas(seen map[string]bool, names ...string) bool {
	for _, name := range names {
		if seen[name] {
			return true
		}
	}
	return false
}

func wrapUnsupported(msg string) error {
	return fmt.Errorf("%w: %s", legacy.ErrUnsupported, msg)
}

func wrapValidation(msg string) error {
	return fmt.Errorf("%w: %s", legacy.ErrValidation, msg)
}

func interspersedFlagsFirst(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if before, _, ok := strings.Cut(name, "="); ok {
			name = before
		}
		flagDef := fs.Lookup(name)
		if flagDef == nil || isBoolFlag(flagDef) || strings.Contains(arg, "=") {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positionals...)
}

func isBoolFlag(flagDef *flag.Flag) bool {
	type boolFlag interface {
		IsBoolFlag() bool
	}
	value, ok := flagDef.Value.(boolFlag)
	return ok && value.IsBoolFlag()
}

func readMessage(value, file string) (string, error) {
	if value != "" && file != "" {
		return "", errors.New("use either inline text or file, not both")
	}
	if file == "" {
		return value, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\n"), nil
}

func readContent(value, file string) ([]byte, error) {
	if value != "" && file != "" {
		return nil, errors.New("use either --content or --content-file, not both")
	}
	if file != "" {
		return os.ReadFile(file)
	}
	if value == "" {
		return nil, errors.New("--content or --content-file is required")
	}
	return []byte(value), nil
}

func oneNumberArg(fs *flag.FlagSet, missingMessage string) (int, error) {
	if fs.NArg() != 1 {
		return 0, errors.New(missingMessage)
	}
	number, err := strconv.Atoi(fs.Arg(0))
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("invalid pull request number %q", fs.Arg(0))
	}
	return number, nil
}

func mergeMethod(merge, squash, rebase bool) (string, error) {
	count := 0
	method := "merge"
	if merge {
		count++
		method = "merge"
	}
	if squash {
		count++
		method = "squash"
	}
	if rebase {
		count++
		method = "rebase"
	}
	if count > 1 {
		return "", errors.New("choose only one of --merge, --squash, or --rebase")
	}
	return method, nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeCommitList(w io.Writer, commits []gitbucket.Commit) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SHA\tAUTHOR\tMESSAGE")
	for _, commit := range commits {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", shortSHA(commit.SHA), commit.Commit.Author.Name, firstLine(commit.Commit.Message))
	}
	_ = tw.Flush()
}

func writeCommitView(w io.Writer, commit *gitbucket.Commit) {
	fmt.Fprintf(w, "commit %s\n", commit.SHA)
	if commit.Commit.Author.Name != "" {
		fmt.Fprintf(w, "Author: %s <%s>\n", commit.Commit.Author.Name, commit.Commit.Author.Email)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, commit.Commit.Message)
}

func writePRList(w io.Writer, prs []gitbucket.PullRequest) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NUMBER\tSTATE\tTITLE\tBRANCH")
	for _, pr := range prs {
		fmt.Fprintf(tw, "#%d\t%s\t%s\t%s -> %s\n", pr.Number, pr.State, pr.Title, pr.Head.Ref, pr.Base.Ref)
	}
	_ = tw.Flush()
}

func writePRView(w io.Writer, pr *gitbucket.PullRequest) {
	fmt.Fprintf(w, "title:\t%s\n", pr.Title)
	fmt.Fprintf(w, "state:\t%s\n", pr.State)
	fmt.Fprintf(w, "number:\t#%d\n", pr.Number)
	fmt.Fprintf(w, "head:\t%s\n", pr.Head.Ref)
	fmt.Fprintf(w, "base:\t%s\n", pr.Base.Ref)
	if pr.HTMLURL != "" {
		fmt.Fprintf(w, "url:\t%s\n", pr.HTMLURL)
	}
	if pr.Body != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, pr.Body)
	}
}

func shortSHA(sha string) string {
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if i := strings.IndexByte(value, '\n'); i >= 0 {
		return value[:i]
	}
	return value
}

func printRootHelp(w io.Writer) {
	fmt.Fprintln(w, `bkt - GitBucket command line tool

Usage:
  bkt [--repo owner/name] <command> [flags]

Commands:
  auth      Manage authentication
  commit    View commits and create single-file commits through Contents API
  issue     Create issues on a legacy GitBucket host
  pr        Work with pull requests

Global flags:
  -R, --repo owner/name  Repository
  -u, --url URL          GitBucket URL
      --json             Output JSON
  -h, --help             Show help
  -v, --version          Show version`)
}

func defaultRunCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func defaultOpenBrowser(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}
