package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(info BuildInfo) *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		Run: func(_ *cobra.Command, _ []string) {
			v := info.Version
			if v == "" {
				v = "dev"
			}
			c := info.Commit
			if c == "" {
				c = "none"
			}
			d := info.Date
			if d == "" {
				d = "unknown"
			}
			fmt.Printf("stabsight version %s (commit: %s, date: %s)\n", v, c, d)
		},
	}

	return versionCmd
}
