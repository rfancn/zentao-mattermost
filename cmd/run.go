package cmd

import (
	"github.com/hdget/sdk"
	logger "github.com/hdget/utils/logger"
	"github.com/rfancn/zentao-mattermost/g"
	"github.com/rfancn/zentao-mattermost/internal/webhook"
	"github.com/spf13/cobra"
)

var argAppAddress string

var cmdRunServer = &cobra.Command{
	Use:   "run",
	Short: "启动 webhook 服务",
	PreRun: func(cmd *cobra.Command, args []string) {
		err := sdk.New(g.App, sdk.WithConfigFile(argConfigFile)).UseConfig(&g.Config).Initialize()
		if err != nil {
			logger.Fatal("初始化 SDK", "err", err)
		}
		if err := g.Config.Normalize(); err != nil {
			logger.Fatal("校验配置", "err", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		runServer()
	},
}

func init() {
	cmdRunServer.PersistentFlags().StringVarP(&argAppAddress, "app-address", "d", "", "服务监听地址，覆盖配置文件中的 server.listen")
	rootCmd.AddCommand(cmdRunServer)
}

func runServer() {
	addr := g.Config.Server.Listen
	if argAppAddress != "" {
		addr = argAppAddress
	}

	webhook.Serve(addr)
}
