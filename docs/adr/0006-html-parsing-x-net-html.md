# HTML フォーム抽出は x/net/html を使い、パーサパッケージに隔離する

4.7.1 のフォーム抽出（`/signin`, `/issues/new`, compare ページ, `/issues/edit/:id`, `/issue_comments/state`）は正規表現でなく `golang.org/x/net/html` の DOM 解析を使う。属性並び・引用・空白・エンティティ・同一ページ内の複数フォームのスコープを正しく扱うため。依存は `internal/legacyhtml` に隔離し、`bkt` 本体の標準ライブラリ方針を維持する。テストは「正しいフォームを抽出」「重複・欠落フィールドを拒否」を検証する。
