# GitBucket CLI 要件定義書

## 1. 概要

GitHub CLI (`gh`) のようなコマンドラインツールの GitBucket 版を作成する。
GitBucket API を利用し、コミット作成やプルリクエスト作成などの操作を CLI から行えるようにする。

**API リファレンス**: https://github.com/gitbucket/gitbucket/wiki/API-WebHook
**対象 GitBucket バージョン**: 4.39

GitBucket 4.39 の API は `/api/v3` 配下にある GitHub API 互換のサブセットであり、GitHub API の全機能は実装されていない。
本ツールは 4.39 で利用可能な Pull Requests、Repositories/Commits、Repositories/Contents、Users API を優先して実装する。

## 2. 実装優先順位

1. **認証・設定機能** (Phase 0)
2. **コミット参照・Contents API による単一ファイルコミット機能** (Phase 1)
3. **プルリクエスト作成・参照・マージ支援機能** (Phase 2)

## 3. 技術仕様

| 項目           | 内容                        |
| -------------- | --------------------------- |
| 実装言語       | Go                          |
| 認証方式       | Personal Access Token (PAT) |
| 出力形式       | gh コマンド準拠             |
| 設定ファイル   | ~/.config/gitbucket-cli/config.json |
| 環境変数       | GITBUCKET_TOKEN, GITBUCKET_URL |
| API ベースパス | /api/v3 |

## 4. コマンド設計

### 4.1 グローバルオプション

```
bkt [flags]
  --help, -h      ヘルプ表示
  --version, -v   バージョン表示
  --repo, -R      リポジトリ指定 (owner/repo 形式)
  --json          JSON 形式で出力
```

### 4.2 認証コマンド (Phase 0)

```
bkt auth login
  --token, -t     Personal Access Token を指定
  --url, -u       GitBucket インスタンスの URL (例: https://gitbucket.example.com)

bkt auth status
  現在の認証状態を表示

bkt auth logout
  認証情報を削除
```

### 4.3 コミットコマンド (Phase 1)

```
bkt commit create [flags]
  --message, -m   コミットメッセージ (必須)
  --file, -F      メッセージをファイルから読み込み
  --path, -p      作成・更新するリポジトリ内パス (必須)
  --content, -c   ファイル内容を文字列で指定
  --content-file  ファイル内容をローカルファイルから読み込み
  --sha           更新対象ファイルの blob SHA (省略時は既存ファイルを自動取得)
  --branch, -b    ブランチ名 (デフォルト: 現在のブランチ)
  --repo, -R      リポジトリ指定

bkt commit list [flags]
  --limit, -L     表示件数 (デフォルト: 20)
  --author, -a    作成者でフィルタ
  --branch, -b    ブランチ指定
  --json          JSON 形式で出力

bkt commit view <sha> [flags]
  --json          JSON 形式で出力
```

GitBucket 4.39 は任意の Git tree/blob/commit を組み立てる完全な Git Database API を提供しないため、
`commit create` は Contents API (`PUT /repos/:owner/:repo/contents/:path`) による単一ファイルの作成・更新として扱う。

### 4.4 プルリクエストコマンド (Phase 2)

```
bkt pr create [flags]
  --title, -t     タイトル (必須)
  --body, -b      本文
  --body-file, -F 本文をファイルから読み込み
  --base, -B      マージ先ブランチ (デフォルト: main/master)
  --head, -H      マージ元ブランチ (デフォルト: 現在のブランチ)
  --repo, -R      リポジトリ指定
  --web, -w       ブラウザで開く

bkt pr list [flags]
  --state, -s     状態でフィルタ (open/closed/all, デフォルト: open)
  --author, -a    作成者でフィルタ
  --assignee, -A  担当者でフィルタ
  --limit, -L     表示件数 (デフォルト: 20)
  --json          JSON 形式で出力

bkt pr view <number> [flags]
  --json          JSON 形式で出力
  --web, -w       ブラウザで開く

bkt pr checkout <number>
  PR ブランチをローカルにチェックアウト

bkt pr merge <number> [flags]
  --merge         マージコミットを作成
  --squash        スクイッシュマージ (GitBucket 側が対応している場合のみ)
  --rebase        リベースマージ (GitBucket 側が対応している場合のみ)
  --delete-branch マージ後ブランチ削除
```

GitBucket 4.39 は GitHub の Draft Pull Request と同等の API を提供しないため、`--draft` は実装対象外とする。

## 5. 設定管理

### 5.1 設定ファイル構成

```json
// ~/.config/gitbucket-cli/config.json
{
  "default_url": "https://gitbucket.example.com",
  "aliases": {
    "work": "https://gitbucket.company.com",
    "personal": "https://gitbucket.private.com"
  }
}
```

### 5.2 認証情報の保存

- MVP では `0600` の設定ファイルに保存し、`GITBUCKET_TOKEN` が設定されている場合は環境変数を優先する。
- OS keyring 連携は将来課題とする。
- 将来的にはトークンをセキュアに保存 (keyring または暗号化ファイル)
- プラットフォームごとの保存先:
  - macOS: Keychain
  - Linux: Secret Service API (GNOME Keyring 等)
  - Windows: Windows Credential Manager

## 6. 出力形式

### 6.1 標準出力 (テキスト)

gh コマンドのスタイルに準拠:
- カラフルな出力 (ターミナル対応時)
- テーブル形式の一覧表示
- インタラクティブなプロンプト

### 6.2 JSON 出力

```
bkt commit list --json
bkt pr view 123 --json
```

標準的な JSON 形式で、フィールド名は snake_case で出力。

## 7. 開発フェーズ

### Phase 0: 基盤構築
- [ ] プロジェクト初期化 (Go モジュール)
- [ ] CLI フレームワーク導入 (cobra または clif)
- [ ] 設定管理機能
- [ ] 認証機能 (auth login/status/logout)

### Phase 1: コミット機能
- [ ] GitBucket API クライアント実装
- [ ] `bkt commit create` コマンド (Contents API による単一ファイル作成・更新)
- [ ] `bkt commit list` コマンド
- [ ] `bkt commit view` コマンド

### Phase 2: プルリクエスト機能
- [ ] `bkt pr create` コマンド
- [ ] `bkt pr list` コマンド
- [ ] `bkt pr view` コマンド
- [ ] `bkt pr checkout` コマンド
- [ ] `bkt pr merge` コマンド

## 8. 非機能要件

| 項目           | 内容                                    |
| -------------- | --------------------------------------- |
| パフォーマンス | API レスポンスを待機中はプログレス表示  |
| エラー処理     | わかりやすいエラーメッセージ (gh 依拠)  |
| テスト         | ユニットテスト、統合テスト              |
| ドキュメント   | README、ヘルプコマンド                  |

## 9. 参考資料

- GitBucket API: https://github.com/gitbucket/gitbucket/wiki/API-WebHook
- GitHub CLI: https://cli.github.com/manual/
- Cobra (Go CLI フレームワーク): https://github.com/spf13/cobra
