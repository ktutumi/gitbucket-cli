# Legacy の pr create は呼び出し側が同一性を固定する

Legacy ホストの `pr create` は `--base` / `--head` / `--base-sha` / `--head-sha` を必須とし、比較ページの hidden fields と一致するときだけ POST する。local / remote の git 照合は呼び出し側の責務であり、CLI は期待値を推測しない。remote が動いていれば比較ページの SHA がずれ、そこで落ちる。非 legacy の `pr create` は従来どおり SHA 不要のままにする。
