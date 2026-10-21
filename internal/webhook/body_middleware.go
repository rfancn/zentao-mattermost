package webhook

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	defaultMaxBodyBytes = 1024 * 1024
	bodyContextKey      = "webhook.body"
)

// bodyMiddleware 读取请求体并存入 context，供后续 middleware 和 handler 使用。
func bodyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(defaultMaxBodyBytes))
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeError(c, http.StatusRequestEntityTooLarge, "body too large")
			} else {
				writeError(c, http.StatusBadRequest, "read body failed")
			}
			c.Abort()
			return
		}
		c.Set(bodyContextKey, body)
	}
}
