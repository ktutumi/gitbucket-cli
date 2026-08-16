# SKILL.md は薄い wrapper に書き換え、CLI 実証後に retire する

Asana 取得・マーカー埋め込み・結合フロー（issue→pr）は SKILL.md に残し、GitBucket フォーム操作を `bkt` 呼び出しに委譲する。移行期間中もエージェントワークフローを壊さない。単一入口のシーケンス規則を wrapper に明記し、同一 owner/repo/キーに対して旧スキルと `bkt` を同時実行しない。CLI の統合テスト（httptest の 4.7.1 fake）が実証された後、SKILL.md を retire する。
