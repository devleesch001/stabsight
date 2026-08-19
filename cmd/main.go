// Package main is the entrypoint for the stabsight application.
package main

import (
	"github.com/devleesch001/stabsight/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.Execute(cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
}
