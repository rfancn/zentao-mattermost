// Package webhook 提供禅道 webhook 的 HTTP 接收端点。
package webhook

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/g"
	"github.com/rfancn/zentao-mattermost/internal/dedupe"
	"github.com/rfancn/zentao-mattermost/internal/errdef"
	"github.com/rfancn/zentao-mattermost/internal/notifier"
)

// New 创建 webhook 的 gin 路由引擎。
func New() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	// 未匹配到 POST 路由时返回 405 而不是 404
	r.HandleMethodNotAllowed = true
	r.POST(g.Config.Server.Path, handle)
	return r
}

func handle(c *gin.Context) {
	if !authorized(c.Request) {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	if !isJSON(c.GetHeader("Content-Type")) {
		writeError(c, http.StatusUnsupportedMediaType, "unsupported media type")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(g.Config.HTTP.MaxBodyBytes))
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(c, http.StatusRequestEntityTooLarge, "body too large")
			return
		}
		writeError(c, http.StatusBadRequest, "read body failed")
		return
	}

	var hook notifier.Webhook
	if err := json.Unmarshal(body, &hook); err != nil {
		sdk.Logger().Error("解析禅道 webhook 失败", "err", err)
		writeError(c, http.StatusBadRequest, "invalid json")
		return
	}

	if hook.ObjectType != "bug" || !shouldNotify(hook.Action) {
		writeOK(c)
		return
	}

	var dedupeKey string
	if g.Config.Dedupe.Enabled {
		dedupeKey = dedupe.Key(body)
		if !dedupe.Begin(dedupeKey) {
			sdk.Logger().Debug("重复的禅道 webhook，跳过", "action", hook.Action)
			writeOK(c)
			return
		}
	}

	err = notifier.Notify(c.Request.Context(), hook)
	if err != nil && g.Config.Dedupe.Enabled {
		// 失败时移除去重记录，允许禅道重试
		dedupe.Cancel(dedupeKey)
	}

	if errors.Is(err, errdef.ErrNoAssignee) {
		writeOK(c)
		return
	}
	if errors.Is(err, errdef.ErrBugIDNotFound) {
		sdk.Logger().Error("无法从 webhook 中提取 Bug ID", "text", hook.Text)
		writeError(c, http.StatusBadRequest, "bug id not found")
		return
	}
	if err != nil {
		sdk.Logger().Error("处理禅道 webhook 失败", "err", err, "action", hook.Action)
		writeError(c, http.StatusInternalServerError, "notification failed")
		return
	}

	writeOK(c)
}

func authorized(r *http.Request) bool {
	expected := g.Config.Server.Token
	if expected == "" {
		return false
	}

	got := ""
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
		got = strings.TrimPrefix(v, "Bearer ")
	}
	if got == "" {
		got = r.Header.Get("X-Webhook-Token")
	}

	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}

func shouldNotify(action string) bool {
	return slices.Contains(g.Config.Notify.Actions, action)
}

func writeOK(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"ok": false, "error": message})
}
