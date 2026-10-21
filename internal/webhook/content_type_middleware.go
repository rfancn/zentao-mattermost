package webhook

import (
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
)

// jsonContentTypeMiddleware 校验请求 Content-Type。
func jsonContentTypeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isJSON(c.GetHeader("Content-Type")) {
			writeError(c, http.StatusUnsupportedMediaType, "unsupported media type")
			c.Abort()
		}
	}
}

func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}
