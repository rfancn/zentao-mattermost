// Package cmd 定义 cobra 命令入口。
package cmd

import (
	"os"

	"github.com/rfancn/zentao-mattermost/g"
	"github.com/spf13/cobra"
)

var argConfigFile string

var rootCmd = &cobra.Command{
	Use:   g.App,
	Short: "禅道 webhook 转 Mattermost 通知服务",
}

// Execute 执行根命令。
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&argConfigFile, "config", "c", "", "配置文件路径，留空时由 hd SDK 自动搜索")
}
