package webhook

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rfancn/zentao-mattermost/config"
)

// authMiddleware 校验请求携带的 token。
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authorized(c) {
			writeError(c, http.StatusUnauthorized, "unauthorized")
			c.Abort()
		}
	}
}

func authorized(c *gin.Context) bool {
	expected := config.Config.App.Token
	if expected == "" {
		return false
	}

	got := ""
	if v := c.GetHeader("Authorization"); strings.HasPrefix(v, "Bearer ") {
		got = strings.TrimPrefix(v, "Bearer ")
	}
	if got == "" {
		got = c.GetHeader("X-Webhook-BotToken")
	}
	if got == "" {
		got = c.Query("token")
	}
	if got == "" {
		got = c.Param("token")
	}

	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}
