# GitBucket CLI 実装計画

## 前提

- 対象 GitBucket は 4.39。
- GitBucket API は `/api/v3` 配下の GitHub API 互換サブセット。
- PRD の `commit create` は、GitBucket 4.39 が対応する Contents API の `Create or update file contents` として実装する。
- 依存関係を増やさず、Go 標準ライブラリで CLI、HTTP クライアント、設定ファイルを実装する。
- 認証トークンは `GITBUCKET_TOKEN` を最優先し、`auth login` では `0600` の設定ファイルへ保存する。OS keyring は将来課題として PRD に明記する。

## タスク

- [x] PRD を GitBucket 4.39 の実装可能範囲に合わせて改善する
- [x] Go モジュールを初期化する
- [x] GitBucket API クライアントの失敗するテストを書く
- [x] API クライアントの最小実装を追加する
- [x] 設定管理と認証コマンドの失敗するテストを書く
- [x] 設定管理と `auth login/status/logout` を実装する
- [x] `commit list/view/create` の失敗するテストを書く
- [x] `commit list/view/create` を実装する
- [x] `pr create/list/view/merge/checkout` の失敗するテストを書く
- [x] `pr create/list/view/merge/checkout` を実装する
- [x] README と作業記録を追加する
- [x] `go test ./...` と `go build ./...` で検証する

## レビュー

- GitBucket 4.39 の API サブセットに合わせ、`commit create` は Contents API による単一ファイル作成・更新として実装した。
- 認証は `GITBUCKET_TOKEN` / `GITBUCKET_URL` を優先し、`auth login` では `0600` の設定ファイルに保存する。
- `go test -count=1 ./...`、`go build ./...`、`go run ./cmd/bkt --version`、`go run ./cmd/bkt --help` で検証した。
