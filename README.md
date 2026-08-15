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
