package cmd

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/hdget/sdk"
	logger "github.com/hdget/utils/logger"
	"github.com/rfancn/zentao-mattermost/g"
	"github.com/rfancn/zentao-mattermost/internal/dedupe"
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

	dedupe.Init()

	srv := &http.Server{
		Addr:              addr,
		Handler:           webhook.New(),
		ReadHeaderTimeout: time.Duration(g.Config.Server.ReadTimeoutSeconds) * time.Second,
		ReadTimeout:       time.Duration(g.Config.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(g.Config.HTTP.TimeoutSeconds+5) * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		sdk.Logger().Info("webhook 服务启动", "address", addr, "path", g.Config.Server.Path)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			sdk.Logger().Fatal("webhook 服务异常退出", "err", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(g.Config.Server.ShutdownTimeoutSeconds)*time.Second,
	)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		sdk.Logger().Error("webhook 服务关闭失败", "err", err)
		return
	}
	sdk.Logger().Info("webhook 服务已关闭")
}
