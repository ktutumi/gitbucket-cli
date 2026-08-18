# GitBucket CLI — Legacy Mode Context

GitBucket の REST API（`/api/v3`, GitHub 互換）を持たない古いインスタンス（4.7.1 など）を、Web フォーム経由で操作する文脈。置き換え対象は `gitbucket-curl-workflow` スキルの curl 手順。

## Language

**Legacy host**（レガシーホスト）:
`/api/v3` を持たない GitBucket インスタンス。明示的に `legacy` と宣言する。自動判定しない。紐づくのはサインイン名であり、token ではない。
_Avoid_: 古い GitBucket, REST なし, token 付きの legacy

**Sign-in name**（サインイン名）:
Legacy host に紐づくユーザー名。ホスト設定の一部であり、パスワードではない。
_Avoid_: 保存されたパスワード, token, アカウント

**Web session**（Web セッション）:
1 コマンドのあいだだけサインインフォームで確立されるセッション。cookie は残さない。
_Avoid_: ログイン, セッション保持, cookie の保存

**Issue**:
GitBucket の Issue エンティティ。番号で参照され、作成・更新・再開の対象となる。
_Avoid_: タイトルからの推測

**Pull request**:
GitBucket の Pull request エンティティ。base/head を固定して作成される。
_Avoid_: 未検証の PR

**Compare page**（比較ページ）:
PR 作成フォームに必要な hidden fields を取得するページ。
_Avoid_: PR フォーム, 手入力の hidden fields

**Raw Markdown**（生マークダウン）:
Issue 編集フォームに含まれる元の Markdown。マーカー走査と検証の対象。
_Avoid_: 表示用レンダリング HTML

**Marker**（マーカー）:
冪等化のため Issue 本文に埋め込まれる識別子。ラベルと値の組であり、1 行として数える。形式は呼び出し側が決め、本文に埋め込む。無い場合は作成しない。
_Avoid_: CLI による注入, マーカー無しでの作成, 値単独の走査, 部分文字列

**Idempotent create**（冪等な作成）:
Legacy の Issue 作成はこれだけである。マーカーを調べてから作成する。既存なら再利用し、この操作の範囲で重複を作らない。不確実記録がある場合は再作成しない。
_Avoid_: 無条件の作成, マーカー無しでの作成, 絶対的な重複防止

**Reuse**（再利用）:
走査で既存マーカーが見つかった場合、新規作成せず既存 Issue を使う。
_Avoid_: 重複作成

**Uncertainty**（不確実性）:
送信後に応答を失った状態。不確実記録を残し、次回の走査で解消を試みる。解消できない場合は人手確認を要する。
_Avoid_: 不明なまま放置, 再送信

**Uncertainty record**（不確実記録）:
POST したかもしれない事実を残す CLI 側の局所状態。存在する限り再作成を抑止する。
_Avoid_: 本文マーカーとの混同, 応答喪失後にだけ残す記録, 記録なしでの再作成

**Override**（明示オーバーライド）:
不確実記録があるときの再作成禁止を外す操作。走査やマーカー検証まで取り消すものではない。
_Avoid_: force, 検証の一括スキップ

**Reconcile**（整合）:
不確実な結果を次回の走査で確定させる。不確実記録が存在する限り再作成しない。走査の不在は不存在の証明にならない。
_Avoid_: 再送信, 不在を失敗と断定, 手動確認なしの自動作成

**Strict identity**（厳格な同一性）:
PR 作成時に呼び出し側が base/head のブランチと full SHA を事前に固定し、比較ページの値と一致することを検証する。CLI は期待値を推測しない。
_Avoid_: 未検証の PR 作成, git からの推測
