package cmd

import (
	"fmt"
	"runtime"

	"github.com/rfancn/zentao-mattermost/config"
	"github.com/spf13/cobra"
)

// 由构建脚本通过 -ldflags -X 注入
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

var cmdVersion = &cobra.Command{
	Use:   "version",
	Short: "打印版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s (commit %s, built %s, %s/%s)\n",
			config.App, version, commit, buildDate, runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	rootCmd.AddCommand(cmdVersion)
}
