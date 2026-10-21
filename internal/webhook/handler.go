// Package webhook 提供 webhook 的 HTTP 接收端点。
package webhook

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/config"
	"github.com/rfancn/zentao-mattermost/internal/notifier"
)

func newHandler() (*gin.Engine, error) {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	// 未匹配到 POST 路由时返回 405 而不是 404
	r.HandleMethodNotAllowed = true

	for name, route := range config.Config.App.Routes {
		n, err := notifier.New(route.Source, route.Target)
		if err != nil {
			return nil, fmt.Errorf("初始化路由 %q: %w", name, err)
		}

		handlers := []gin.HandlerFunc{
			authMiddleware(),
			jsonContentTypeMiddleware(),
			bodyMiddleware(),
			dedupeMiddleware(),
			handle(n),
		}

		path := "/" + name
		r.POST(path, handlers...)
		// 上游无法自定义请求头时，可通过路径参数传递 token
		r.POST(path+"/:token", handlers...)
	}
	return r, nil
}

// handle 将请求体交给对应的 Notifier 处理。
func handle(n notifier.Notifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := c.MustGet(bodyContextKey).([]byte)
		if err := n.Notify(c.Request.Context(), body); err != nil {
			sdk.Logger().Error("处理 webhook 失败", "err", err)
			writeError(c, http.StatusInternalServerError, err.Error())
			return
		}
		writeOK(c)
	}
}

func writeOK(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"ok": false, "error": message})
}
