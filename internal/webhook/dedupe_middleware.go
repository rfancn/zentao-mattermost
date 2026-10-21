package webhook

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hdget/sdk"
)

const (
	dedupeTTL        = 10 * time.Minute
	dedupeMaxEntries = 2048
)

// dedupeStore 是进程内的 webhook 去重存储。
type dedupeStore struct {
	mu         sync.Mutex
	entries    map[string]time.Time
	ttl        time.Duration
	maxEntries int
}

func newDedupeStore() *dedupeStore {
	return &dedupeStore{
		entries:    make(map[string]time.Time),
		ttl:        dedupeTTL,
		maxEntries: dedupeMaxEntries,
	}
}

var dedupes = newDedupeStore()

// dedupeMiddleware 按请求体哈希去重；处理失败时移除去重键以便上游重试。
func dedupeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		body := c.MustGet(bodyContextKey).([]byte)
		key := dedupeHash(body)
		if !dedupes.begin(key) {
			sdk.Logger().Debug("重复的 webhook，跳过")
			writeOK(c)
			c.Abort()
			return
		}

		defer func() {
			if r := recover(); r != nil {
				dedupes.cancel(key)
				panic(r)
			}
			if c.Writer.Status() >= http.StatusInternalServerError {
				dedupes.cancel(key)
			}
		}()

		c.Next()
	}
}

// dedupeHash 计算 webhook 请求体的 SHA-256。
func dedupeHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// begin 尝试登记一个去重键。
// 返回 true 表示可以继续处理；返回 false 表示重复，应跳过。
func (s *dedupeStore) begin(key string) bool {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, t := range s.entries {
		if now.Sub(t) >= s.ttl {
			delete(s.entries, k)
		}
	}

	if _, exists := s.entries[key]; exists {
		return false
	}

	if len(s.entries) >= s.maxEntries {
		var oldestKey string
		var oldest time.Time
		for k, t := range s.entries {
			if oldestKey == "" || t.Before(oldest) {
				oldestKey, oldest = k, t
			}
		}
		if oldestKey != "" {
			delete(s.entries, oldestKey)
		}
	}

	s.entries[key] = now
	return true
}

// cancel 在通知失败时移除去重键，允许上游重试。
func (s *dedupeStore) cancel(key string) {
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
}
