# Legacy ホストはサインイン名だけ残し、パスワードと cookie は残さない

`auth login --legacy` が永続するのは `HostConfig.legacy` とサインイン名だけにする。サインイン名は login 時にだけ決まり、以後のコマンドは保存値を使う。パスワードは `--password` または `GITBUCKET_PASSWORD` でコマンドごとに渡し、無い、または空なら認証失敗とする。prompt しない。Web session の cookie はプロセス内に閉じる。パスワード保存は PAT をやめた意味を薄め、cookie 永続は期限切れと権限管理を増やす。prompt はエージェントから使えず、TTY 判定だけが増える。
