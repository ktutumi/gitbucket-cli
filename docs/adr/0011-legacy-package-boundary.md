# Legacy トランスポートは cli と api/v3 クライアントの外に置く

Web session・フォーム POST・Issue 走査のオーケストレーションは `internal/legacy` に置く。`internal/cli` は設定解決とフラグだけを持ち、`internal/gitbucket` は `/api/v3` のままにする。`gitbucket` に混ぜると legacy 判定がクライアント内部に漏れ、`cli` 直書きはテスト境界が溶ける。
