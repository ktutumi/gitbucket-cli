# GitBucket CLI 実装計画

> 更新日時: 2026-09-05 20:30

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

## legacy ホスト実機検証（GitBucket 4.7.1）

- 動作中の GitBucket 4.7.1（`/api/v3` 無し）で legacy 系コマンドを一通り実行した。テスト用リポジトリ `test/bkt-test` を作成し、テスト後に削除済み。

### 確認できた動作

- `auth login --legacy` / `auth status` / `auth logout`: パスワードは保存されない。legacy login は `/signin` への POST を行わない。
- `issue create`: 作成（exit 0）、マーカー行が一致した際の再利用（exit 0 / `reused: true`）、多一致（exit 3）、body にマーカー行が 2 本 / 0 本・`--marker-label` / `--dedupe-key` / `--title` 欠落（いずれも exit 5）。
- 不確実性レコード: レコード存在かつ一致 issue 無しで exit 2、「uncertainty record exists and no matching issue was found」。`--override-uncertainty` で作成が進む。作成成功後にレコードがクリアされる。
- `pr create`: 作成（exit 0）と `--json`。`--head-sha` 欠落（exit 5）、compare ページの hidden fields と caller-fixed SHA の不一致（exit 5、fail-closed）。
- legacy ホストでの非対応コマンド（`commit list` / `pr list` / `pr view` / `pr merge` / `pr checkout`）: 全て exit 1。
- 非 legacy ホストでの `issue create`（exit 1）と `pr create --base-sha/--head-sha`（exit 1）のガード。

### 検出された問題

1. **[重要] legacy の SignIn が誤ったパスワードでも成功とみなす**（`internal/legacy/client.go` の `SignIn`）。GitBucket 4.7.1 の `/signin` は認証失敗でも 302 を返す（Location が `/` ではなく `/signin`）。現在 `StatusCode == 302 && Location != ""` だけで成功と判定するため、誤パスワードで exit 4（認証失敗）にならず、exit 1（「issue #N raw markdown failed: HTTP 401」）や exit 2（不確実性）になる。不確実性レコードが誤って残る可能性もある。修正案: 302 の Location が `/signin` 側（再提示）に戻る場合は `ErrAuth` とみなす。
2. **[要検討] `--base-sha` の意味が README に無い**。GitBucket 4.7.1 の compare フォームの `commitIdFrom` は base 先端 SHA ではなく merge-base。分岐が進んだブランチ間で base 先端 SHA を渡すと「compare page identity does not match」で必ず失敗する。ADR-0010 により呼び出し側の責務ではあるが、README に `--base-sha` は compare ページの `commitIdFrom`（merge-base）に一致する値であることを明記すべき。
3. **[軽微] unsupported メッセージの二重表示**。「legacy: command is not supported: command is not supported on a legacy GitBucket host」。`wrapUnsupported` と `legacy.ErrUnsupported` が同じ表現を含むため。

### 指摘事項の修正・再検証

- [x] SignIn の 302 Location を URL として解決し、コンテキストルート配下の signin への戻りを認証失敗とする。相対 URL・絶対 URL・クエリ付き・末尾スラッシュ付きも回帰テストで確認した。
- [x] README と CLI ヘルプに、`--base-sha` は compare フォームの `commitIdFrom`（GitBucket 4.7.1 では merge-base）であることを明記した。
- [x] legacy 非対応コマンドのエラーメッセージの重複を除去した。
- [x] 回帰テストが修正前に失敗し、修正後に成功することを確認した。`go test -count=1 ./...`、`go build ./...`、`go vet ./...` が成功した（ビルドキャッシュは `/tmp` 配下を使用）。
- [x] 指定の使い捨て環境で、誤パスワードによる Issue・PR 作成が exit 4 となり、不確実性レコードが残らないことを確認した。正しいパスワードでは Issue 作成・再利用が exit 0、非対応コマンドは exit 1 でメッセージが重複しないことを確認した。
- [x] 今回作成した検証用リポジトリ `test/bkt-signin-regression` を削除した。
