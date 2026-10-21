package webhook

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/config"
)

const (
	defaultReadTimeoutSec  = 10
	defaultWriteTimeoutSec = 15
	defaultShutdownSec     = 10
)

// Serve 启动 webhook HTTP 服务，并在收到退出信号后优雅关闭。
func Serve(addr string) {
	handler, err := newHandler()
	if err != nil {
		sdk.Logger().Fatal("初始化 webhook 路由失败", "err", err)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: time.Duration(defaultReadTimeoutSec) * time.Second,
		ReadTimeout:       time.Duration(defaultReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(defaultWriteTimeoutSec) * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	routeNames := make([]string, 0, len(config.Config.App.Routes))
	for name := range config.Config.App.Routes {
		routeNames = append(routeNames, name)
	}
	slices.Sort(routeNames)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		sdk.Logger().Info("webhook 服务启动", "address", addr)
		for _, name := range routeNames {
			route := config.Config.App.Routes[name]
			sdk.Logger().Info("webhook 路由已监听",
				"url", "/"+name,
				"sourceType", route.Source.Type,
				"sourceUrl", route.Source.URL,
				"targetType", route.Target.Type,
				"targetUrl", route.Target.URL,
			)
		}
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			sdk.Logger().Fatal("webhook 服务异常退出", "err", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(defaultShutdownSec)*time.Second,
	)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		sdk.Logger().Error("webhook 服务关闭失败", "err", err)
		return
	}
	sdk.Logger().Info("webhook 服务已关闭")
}
