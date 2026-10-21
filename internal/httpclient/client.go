// Package httpclient 提供全局共享的 resty 客户端。
package httpclient

import (
	"crypto/tls"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	defaultHTTPTimeoutSec = 10
)

// New 返回全局共享的 REST 客户端，首次调用时根据配置初始化。
var New = sync.OnceValue(newClient)

func newClient() *resty.Client {
	timeout := time.Duration(defaultHTTPTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return resty.New().
		SetTimeout(timeout).
		SetTLSClientConfig(&tls.Config{
			// #nosec G402 -- 由配置显式开启，用于访问内网自签名证书
			InsecureSkipVerify: false, //nolint:gosec
			MinVersion:         tls.VersionTLS12,
		}).
		SetHeader("Accept", "application/json")
}
