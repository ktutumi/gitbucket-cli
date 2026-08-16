# Legacy の pr create は独立型とする

`bkt pr create` は Issue との結合を要求しない（gh 流儀）。`Closes #N` を本文に含めるかは呼び出し側が決める。スキルの結合フロー（issue 作成 → pr 作成）は wrapper 側で `bkt issue create` → `bkt pr create --body "Closes #N"` として再現する。CLI は GitBucket クライアントであり、ワークフロー結合は CLI の責務としない。
