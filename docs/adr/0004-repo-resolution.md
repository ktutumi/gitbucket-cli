# owner/repo は明示指定を優先、git remote をフォールバック

リポジトリ解決は `--repo owner/name` と `GITBUCKET_REPO` を常に優先し、両方無い場合のみ workdir の `git remote get-url origin` から導出する。スキルの checkout 前提の挙動を CLI 側の明示指定可能な形に置き換え、フォールバックは便宜のため残す。
