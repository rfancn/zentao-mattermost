package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/config"
	"github.com/rfancn/zentao-mattermost/internal/errdef"
	"github.com/rfancn/zentao-mattermost/internal/notifier"
)

type countingNotifier struct {
	calls atomic.Int32
	err   error
}

func (n *countingNotifier) Notify(context.Context, []byte) error {
	n.calls.Add(1)
	return n.err
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "webhook-test")
	if err != nil {
		panic(err)
	}

	content := fmt.Sprintf(`
[sdk.log]
level = "error"
filename = "test.log"
dir = %q
[sdk.log.rotate]
max_age = 1
max_backup = 1
max_size = 1
compress = false
`, dir)
	if err := sdk.New("webhook-test", sdk.WithConfigContent([]byte(content))).Initialize(); err != nil {
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

const testRouteName = "test-route"

func testRoute() config.RouteConfig {
	return config.RouteConfig{
		Source: config.SourceConfig{Type: "zentao", URL: "http://zentao", Account: "admin", Password: "password"},
		Target: config.TargetConfig{Type: "mattermost", URL: "http://mattermost", BotToken: "mm"},
	}
}

func testConfig() config.AppConfig {
	return config.AppConfig{
		Token: "secret",
		Routes: map[string]config.RouteConfig{
			testRouteName: testRoute(),
		},
	}
}

func TestAuthorizedTokenSources(t *testing.T) {
	config.Config = config.RootConfig{App: testConfig()}

	handler, err := newHandler()
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	path := "/test-route"

	tests := []struct {
		name    string
		target  string
		headers map[string]string
		want    int
	}{
		{
			name:    "authorization header",
			target:  path,
			headers: map[string]string{"Authorization": "Bearer secret"},
			want:    http.StatusOK,
		},
		{
			name:    "x-webhook-token header",
			target:  path,
			headers: map[string]string{"X-Webhook-BotToken": "secret"},
			want:    http.StatusOK,
		},
		{
			name:   "query parameter",
			target: path + "?token=secret",
			want:   http.StatusOK,
		},
		{
			name:   "path parameter",
			target: path + "/secret",
			want:   http.StatusOK,
		},
		{
			name:   "missing token",
			target: path,
			want:   http.StatusUnauthorized,
		},
		{
			name:   "wrong query token",
			target: path + "?token=wrong",
			want:   http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.target, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("状态码 = %d, want %d, body = %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestNewHandlerUnknownSourceType(t *testing.T) {
	cfg := testConfig()
	route := cfg.Routes[testRouteName]
	route.Source.Type = "unknown"
	cfg.Routes[testRouteName] = route
	config.Config = config.RootConfig{App: cfg}

	if _, err := newHandler(); !errors.Is(err, errdef.ErrSourceTypeUnknown) {
		t.Fatalf("newHandler() error = %v, want ErrSourceTypeUnknown", err)
	}
}

func TestNewHandlerUnknownTargetType(t *testing.T) {
	cfg := testConfig()
	route := cfg.Routes[testRouteName]
	route.Target.Type = "unknown"
	cfg.Routes[testRouteName] = route
	config.Config = config.RootConfig{App: cfg}

	if _, err := newHandler(); !errors.Is(err, errdef.ErrTargetTypeUnknown) {
		t.Fatalf("newHandler() error = %v, want ErrTargetTypeUnknown", err)
	}
}

func newTestEngine(n notifier.Notifier) *gin.Engine {
	r := gin.New()
	r.POST("/test-route",
		authMiddleware(),
		jsonContentTypeMiddleware(),
		bodyMiddleware(),
		dedupeMiddleware(),
		handle(n),
	)
	return r
}

func postWebhook(t *testing.T, handler http.Handler, body string) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/test-route", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}

func TestDuplicateWebhookSkipped(t *testing.T) {
	config.Config = config.RootConfig{App: testConfig()}
	dedupes = newDedupeStore()

	n := &countingNotifier{}
	r := newTestEngine(n)

	for i := 0; i < 2; i++ {
		if code := postWebhook(t, r, `{}`); code != http.StatusOK {
			t.Fatalf("第 %d 次状态码 = %d, want 200", i+1, code)
		}
	}
	if got := n.calls.Load(); got != 1 {
		t.Errorf("Notify 调用次数 = %d, want 1", got)
	}
}

func TestFailedNotifyAllowsRetry(t *testing.T) {
	config.Config = config.RootConfig{App: testConfig()}
	dedupes = newDedupeStore()

	n := &countingNotifier{err: errors.New("boom")}
	r := newTestEngine(n)

	for i := 0; i < 2; i++ {
		if code := postWebhook(t, r, `{}`); code != http.StatusInternalServerError {
			t.Fatalf("第 %d 次状态码 = %d, want 500", i+1, code)
		}
	}
	if got := n.calls.Load(); got != 2 {
		t.Errorf("Notify 调用次数 = %d, want 2", got)
	}
}
