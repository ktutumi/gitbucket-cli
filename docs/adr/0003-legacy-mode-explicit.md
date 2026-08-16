# Legacy モードは設定で明示、自動判定しない

ホストが Legacy（`/api/v3` 無し）かどうかは設定ファイルの `HostConfig.legacy: true` で明示する。`/api/v3` のプローブによる自動判定は行わない。理由: 自動判定は起動ごとの追加リクエスト、誤判定（タイムアウト・プロキシ）による不確定性を生む。`auth login --url ... --legacy` で保存し、legacy ホストには token を保存しない。`auth status` は legacy ホストに対し API 検証をスキップし「操作時のサインインフォームで検証」と表示する。
