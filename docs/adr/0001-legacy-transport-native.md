# Legacy トランスポートは curl ラップでなく Go ネイティブ実装

Legacy モード（GitBucket 4.7.1 等、`/api/v3` を持たないインスタンス）の Web フォーム操作は、スキルの Bash/curl ブロックをそのまま wrap するのではなく、Go の `net/http` で同じセマンティクス（`application/x-www-form-urlencoded` 送信、cookie セッション、302+Location 判定）を実装する。curl の存在依存・シェルエスケープ・プラットフォーム差をなくし、`bkt` の標準ライブラリ方針とテスト可能性（`httptest`）を保つため。成功判定は常に 302 + `Location` header のみで行う（Iron Law）。サインインは 302 + 空でない Location 以外を認証失敗とし、Location の行き先は問わない。PR は `/$owner/$repo/pull/<数字>`、Issue は `/$owner/$repo/issues/<数字>` だけを成功とする。それ以外の Location や応答喪失は再 POST しない。
