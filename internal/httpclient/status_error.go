package httpclient

import (
	"errors"
	"fmt"
)

// StatusError 表示上游返回的非 2xx 响应。
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, truncate(e.Body, 500))
}

// IsStatus 判断错误是否为指定状态码的 StatusError。
func IsStatus(err error, code int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.StatusCode == code
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
