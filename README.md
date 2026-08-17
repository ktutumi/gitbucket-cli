# gitbucket-cli

`gitbucket-cli` provides a `gh`-style command line interface for GitBucket 4.39.

The binary name is `bkt`. It talks to GitBucket's GitHub-compatible API subset under `/api/v3`.

## Install

```sh
go build -o bkt ./cmd/bkt
```

## Configuration

Authenticate with a GitBucket URL and a Personal Access Token:

```sh
bkt auth login --url https://gitbucket.example.com --token "$GITBUCKET_TOKEN"
bkt auth status
```

Configuration is stored in:

```text
~/.config/gitbucket-cli/config.json
```

Environment variables override the configuration file:

```sh
export GITBUCKET_URL=https://gitbucket.example.com
export GITBUCKET_TOKEN=...
export GITBUCKET_REPO=owner/repo
export GITBUCKET_PASSWORD=...   # legacy hosts only; never stored
```

## Commands

```sh
bkt --repo owner/repo commit list --branch main --limit 20
bkt --repo owner/repo commit view <sha> --json
```

`commit create` uses the GitBucket 4.39 Contents API, so it creates or updates one file per command:

```sh
bkt --repo owner/repo commit create \
  --message "docs: update readme" \
  --path README.md \
  --content-file README.md \
  --branch main
```

Pull request commands:

```sh
bkt --repo owner/repo pr create --title "Add API" --body "Summary" --head feature/api --base main
bkt --repo owner/repo pr list --state open
bkt --repo owner/repo pr view 12 --json
bkt --repo owner/repo pr checkout 12
bkt --repo owner/repo pr merge 12 --merge --delete-branch
```

GitBucket 4.39 does not provide a Draft Pull Request API equivalent, so `--draft` is intentionally not supported.

Repository resolution prefers `--repo owner/name`, then `GITBUCKET_REPO`, then `git remote get-url origin` (HTTPS, SSH, and SCP-style URLs).

## Legacy hosts

Declare a host that has no usable `/api/v3`. Login stores the sign-in name only and does not POST `/signin`:

```sh
bkt auth login --url https://legacy.example.com --legacy --user root
bkt auth status
```

Password is supplied per command (`--password` or `GITBUCKET_PASSWORD`). Supported commands:

```sh
bkt --repo owner/repo issue create \
  --title "Task" --body-file body.md \
  --marker-label Asana-Task-ID --dedupe-key 99 \
  --password "$GITBUCKET_PASSWORD"

bkt --repo owner/repo pr create \
  --title "Add API" --body "Summary" \
  --base main --head feature/api \
  --base-sha <40-hex> --head-sha <40-hex> \
  --password "$GITBUCKET_PASSWORD"
```

`--base`, `--head`, `--base-sha`, and `--head-sha` are required on a legacy host. Short aliases `-B` / `-H` count. Other commands (commit, `pr list` / `view` / `merge` / `checkout`) are unsupported.

Legacy exit codes:

| Code | Meaning |
|------|---------|
| 0 | success |
| 1 | generic / unsupported |
| 2 | issue creation is uncertain |
| 3 | multiple matching issues |
| 4 | authentication failed |
| 5 | validation failed |
