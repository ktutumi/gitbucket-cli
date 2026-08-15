package main

import (
	"context"
	"os"

	"github.com/ktutumi/gitbucket-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], cli.Options{}))
}
