package zentao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/config"
	"github.com/rfancn/zentao-mattermost/internal/errdef"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "zentao-client-test")
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
	if err := sdk.New("zentao-client-test", sdk.WithConfigContent([]byte(content))).Initialize(); err != nil {
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func newSource(t *testing.T, url string) *Source {
	t.Helper()

	src, err := NewSource(config.SourceConfig{URL: url, Account: "admin", Password: "password"})
	if err != nil {
		t.Fatalf("NewSource() error = %v", err)
	}
	return src
}

func TestNewSourceValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.SourceConfig
	}{
		{name: "missing url", cfg: config.SourceConfig{Account: "admin", Password: "password"}},
		{name: "missing account", cfg: config.SourceConfig{URL: "http://zentao", Password: "password"}},
		{name: "missing password", cfg: config.SourceConfig{URL: "http://zentao", Account: "admin"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewSource(tt.cfg); err == nil {
				t.Fatal("NewSource() error = nil, want error")
			}
		})
	}
}

func bugServer(tokenCalls, bugCalls *atomic.Int32, expectedToken string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api.php/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		call := tokenCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": fmt.Sprintf("%s-%d", expectedToken, call)})
	})
	mux.HandleFunc("/api.php/v1/bugs/3", func(w http.ResponseWriter, r *http.Request) {
		bugCalls.Add(1)
		if r.Header.Get("BotToken") != expectedToken+"-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(Bug{
			ID:         3,
			Title:      "test_bug3",
			AssignedTo: &User{ID: 2, Account: "testuser"},
		})
	})
	return httptest.NewServer(mux)
}

func TestParseWithCredentials(t *testing.T) {
	var tokenCalls, bugCalls atomic.Int32
	srv := bugServer(&tokenCalls, &bugCalls, "token")
	defer srv.Close()

	src := newSource(t, srv.URL)

	msg, err := src.Parse(context.Background(), []byte(`{"objectType":"bug","objectID":"3","action":"edited","text":"x"}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if msg == nil || msg["recipient"] != "testuser" {
		t.Fatalf("Parse() = %+v", msg)
	}
	if !strings.Contains(msg["text"], "Bug #3：test_bug3") {
		t.Errorf("消息内容 = %q", msg["text"])
	}
	if got := tokenCalls.Load(); got != 1 {
		t.Errorf("token 请求次数 = %d, want 1", got)
	}
}

func TestTokenIsCached(t *testing.T) {
	var tokenCalls, bugCalls atomic.Int32
	srv := bugServer(&tokenCalls, &bugCalls, "cached")
	defer srv.Close()

	src := newSource(t, srv.URL)
	body := []byte(`{"objectType":"bug","objectID":"3","action":"edited","text":"x"}`)

	for i := 0; i < 2; i++ {
		if _, err := src.Parse(context.Background(), body); err != nil {
			t.Fatalf("第 %d 次 Parse() error = %v", i+1, err)
		}
	}
	if got := tokenCalls.Load(); got != 1 {
		t.Errorf("token 请求次数 = %d, want 1（应命中缓存）", got)
	}
	if got := bugCalls.Load(); got != 2 {
		t.Errorf("Bug 请求次数 = %d, want 2", got)
	}
}

func TestParseRefreshesTokenOnUnauthorized(t *testing.T) {
	var tokenCalls, bugCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api.php/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		call := tokenCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": fmt.Sprintf("token-%d", call)})
	})
	mux.HandleFunc("/api.php/v1/bugs/3", func(w http.ResponseWriter, r *http.Request) {
		bugCalls.Add(1)
		if r.Header.Get("BotToken") != "token-2" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(Bug{ID: 3, Title: "ok", AssignedTo: &User{ID: 2, Account: "testuser"}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	src := newSource(t, srv.URL)

	msg, err := src.Parse(context.Background(), []byte(`{"objectType":"bug","objectID":"3","action":"edited","text":"x"}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if msg == nil || !strings.Contains(msg["text"], "Bug #3：ok") {
		t.Fatalf("Parse() = %+v", msg)
	}
	if got := tokenCalls.Load(); got != 2 {
		t.Errorf("token 请求次数 = %d, want 2", got)
	}
	if got := bugCalls.Load(); got != 2 {
		t.Errorf("Bug 请求次数 = %d, want 2", got)
	}
}

func TestParseSkipsNonBug(t *testing.T) {
	src := newSource(t, "http://127.0.0.1:1")

	msg, err := src.Parse(context.Background(), []byte(`{"objectType":"story","action":"edited"}`))
	if err != nil || msg != nil {
		t.Fatalf("Parse() = %+v, err = %v; want nil, nil", msg, err)
	}
}

func TestParseInvalidWebhook(t *testing.T) {
	src := newSource(t, "http://127.0.0.1:1")

	_, err := src.Parse(context.Background(), []byte(`{`))
	if !errors.Is(err, errdef.ErrInvalidWebhook) {
		t.Fatalf("Parse() error = %v, want ErrInvalidWebhook", err)
	}
}

func TestParseMissingBugID(t *testing.T) {
	src := newSource(t, "http://127.0.0.1:1")

	_, err := src.Parse(context.Background(), []byte(`{"objectType":"bug","objectID":"","action":"edited","text":"no id"}`))
	if !errors.Is(err, errdef.ErrBugIDNotFound) {
		t.Fatalf("Parse() error = %v, want ErrBugIDNotFound", err)
	}
}

func TestRefreshTokenEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	src := newSource(t, srv.URL)

	if _, err := src.getBug(context.Background(), "3"); err == nil {
		t.Fatal("getBug() error = nil, want error")
	}
}

func TestFormatMessage(t *testing.T) {
	hook := Webhook{
		Action: "edited",
		Actor:  "admin",
		Date:   "2026-10-08 17:43:58",
		Text:   "admin编辑了Bug",
	}
	bug := &Bug{
		ID:          3,
		Title:       "test_bug3",
		ProductName: "产品A",
		Severity:    2,
		Pri:         1,
		Status:      "active",
	}

	got := formatMessage(hook, bug)
	for _, want := range []string{
		"Bug #3：test_bug3",
		"产品：产品A",
		"严重程度：2",
		"优先级：1",
		"状态：active",
		"操作人：admin",
		"操作：edited",
		"时间：2026-10-08 17:43:58",
		"admin编辑了Bug",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("消息缺少 %q:\n%s", want, got)
		}
	}
}
