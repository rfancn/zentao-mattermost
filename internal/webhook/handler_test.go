package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/g"
	"github.com/rfancn/zentao-mattermost/internal/dedupe"
)

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

type fakeEnv struct {
	zenTaoStatus int
	hasAssignee  bool

	mu    sync.Mutex
	posts []map[string]string

	ztSrv *httptest.Server
	mmSrv *httptest.Server
}

func newFakeEnv(t *testing.T) *fakeEnv {
	t.Helper()

	env := &fakeEnv{zenTaoStatus: http.StatusOK, hasAssignee: true}

	ztMux := http.NewServeMux()
	ztMux.HandleFunc("/api.php/v1/bugs/3", func(w http.ResponseWriter, r *http.Request) {
		if env.zenTaoStatus != http.StatusOK {
			w.WriteHeader(env.zenTaoStatus)
			return
		}
		bug := map[string]any{
			"id":          3,
			"title":       "test_bug3",
			"severity":    2,
			"pri":         1,
			"status":      "active",
			"productName": "产品A",
		}
		if env.hasAssignee {
			bug["assignedTo"] = map[string]any{
				"id":       2,
				"account":  "testuser",
				"realname": "李涛",
			}
		}
		_ = json.NewEncoder(w).Encode(bug)
	})
	env.ztSrv = httptest.NewServer(ztMux)

	mmMux := http.NewServeMux()
	mmMux.HandleFunc("/api/v4/users/username/testuser", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "u1", "username": "testuser"})
	})
	mmMux.HandleFunc("/api/v4/users/username/zentao-bot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "bot1", "username": "zentao-bot"})
	})
	mmMux.HandleFunc("/api/v4/teams/team1/channels/name/zentao-testuser", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "c1", "name": "zentao-testuser"})
	})
	mmMux.HandleFunc("/api/v4/channels/c1/members/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	mmMux.HandleFunc("/api/v4/posts", func(w http.ResponseWriter, r *http.Request) {
		var p map[string]string
		_ = json.NewDecoder(r.Body).Decode(&p)
		env.mu.Lock()
		env.posts = append(env.posts, p)
		env.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p1"}`))
	})
	env.mmSrv = httptest.NewServer(mmMux)

	t.Cleanup(func() {
		env.ztSrv.Close()
		env.mmSrv.Close()
	})

	return env
}

func (e *fakeEnv) configure(t *testing.T, maxBodyBytes int) {
	t.Helper()

	if maxBodyBytes <= 0 {
		maxBodyBytes = 1024 * 1024
	}

	g.Config = g.AppConfig{
		Server: g.ServerConfig{
			Listen: ":0",
			Path:   "/webhook",
			Token:  "secret",
		},
		Zentao: g.ZentaoConfig{
			URL:   e.ztSrv.URL,
			Token: "zt",
		},
		Mattermost: g.MattermostConfig{
			URL:     e.mmSrv.URL,
			Token:   "mm-token",
			TeamID:  "team1",
			BotUser: "zentao-bot",
		},
		Channel: g.ChannelConfig{Prefix: "zentao-", Type: "P"},
		Notify:  g.NotifyConfig{Actions: []string{"created", "edited"}},
		Dedupe:  g.DedupeConfig{Enabled: true, TTLSeconds: 600, MaxEntries: 100},
		HTTP:    g.HTTPConfig{TimeoutSeconds: 5, MaxBodyBytes: maxBodyBytes},
	}
	if err := g.Config.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	dedupe.Init()
}

func (e *fakeEnv) request(t *testing.T, method, contentType, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, "http://example.com/webhook", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, req)
	return rec
}

func (e *fakeEnv) postCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.posts)
}

func (e *fakeEnv) lastPost() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.posts) == 0 {
		return nil
	}
	return e.posts[len(e.posts)-1]
}

func validBody() string {
	return `{"objectType":"bug","objectID":"3","action":"edited","actor":"admin","date":"2026-10-08 17:43:58","text":"admin编辑了Bug [#3::test_bug3](http://x/index.php?m=bug&f=view&id=3)"}`
}

func TestUnauthorized(t *testing.T) {
	env := newFakeEnv(t)
	env.configure(t, 0)

	rec := env.request(t, http.MethodPost, "application/json", validBody(), map[string]string{"Authorization": ""})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, want 401", rec.Code)
	}

	rec = env.request(t, http.MethodPost, "application/json", validBody(), map[string]string{"Authorization": "Bearer wrong"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误 token 状态码 = %d, want 401", rec.Code)
	}

	rec = env.request(t, http.MethodPost, "application/json", validBody(), map[string]string{
		"Authorization":   "",
		"X-Webhook-Token": "secret",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("X-Webhook-Token 状态码 = %d, want 200", rec.Code)
	}
}

func TestMethodAndContentType(t *testing.T) {
	env := newFakeEnv(t)
	env.configure(t, 0)

	rec := env.request(t, http.MethodGet, "", "", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 状态码 = %d, want 405", rec.Code)
	}

	rec = env.request(t, http.MethodPost, "text/plain", validBody(), nil)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain 状态码 = %d, want 415", rec.Code)
	}
}

func TestInvalidJSON(t *testing.T) {
	env := newFakeEnv(t)
	env.configure(t, 0)

	rec := env.request(t, http.MethodPost, "application/json", `{`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, want 400", rec.Code)
	}
}

func TestSkipNonBugAndUnwatchedAction(t *testing.T) {
	env := newFakeEnv(t)
	env.configure(t, 0)

	nonBug := `{"objectType":"story","objectID":"3","action":"edited","text":"x"}`
	if rec := env.request(t, http.MethodPost, "application/json", nonBug, nil); rec.Code != http.StatusOK {
		t.Fatalf("非 Bug 状态码 = %d, want 200", rec.Code)
	}

	unwatched := `{"objectType":"bug","objectID":"3","action":"deleted","text":"x"}`
	if rec := env.request(t, http.MethodPost, "application/json", unwatched, nil); rec.Code != http.StatusOK {
		t.Fatalf("未订阅动作状态码 = %d, want 200", rec.Code)
	}

	if env.postCount() != 0 {
		t.Errorf("跳过场景不应发送消息，实际 %d 条", env.postCount())
	}
}

func TestNotifySuccess(t *testing.T) {
	env := newFakeEnv(t)
	env.configure(t, 0)

	rec := env.request(t, http.MethodPost, "application/json", validBody(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if env.postCount() != 1 {
		t.Fatalf("消息数量 = %d, want 1", env.postCount())
	}

	p := env.lastPost()
	if p["channel_id"] != "c1" {
		t.Errorf("channel_id = %q", p["channel_id"])
	}
	for _, want := range []string{"@testuser", "Bug #3：test_bug3", "产品：产品A"} {
		if !strings.Contains(p["message"], want) {
			t.Errorf("消息缺少 %q:\n%s", want, p["message"])
		}
	}
}

func TestNoAssigneeSkips(t *testing.T) {
	env := newFakeEnv(t)
	env.hasAssignee = false
	env.configure(t, 0)

	rec := env.request(t, http.MethodPost, "application/json", validBody(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200", rec.Code)
	}
	if env.postCount() != 0 {
		t.Errorf("无指派对象时不应发送消息")
	}
}

func TestZenTaoFailureReturns500(t *testing.T) {
	env := newFakeEnv(t)
	env.zenTaoStatus = http.StatusInternalServerError
	env.configure(t, 0)

	rec := env.request(t, http.MethodPost, "application/json", validBody(), nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d, want 500", rec.Code)
	}
	if env.postCount() != 0 {
		t.Errorf("禅道失败时不应发送消息")
	}
}

func TestDuplicateWebhookSkipped(t *testing.T) {
	env := newFakeEnv(t)
	env.configure(t, 0)

	body := validBody()
	if rec := env.request(t, http.MethodPost, "application/json", body, nil); rec.Code != http.StatusOK {
		t.Fatalf("首次状态码 = %d", rec.Code)
	}
	if rec := env.request(t, http.MethodPost, "application/json", body, nil); rec.Code != http.StatusOK {
		t.Fatalf("重复状态码 = %d", rec.Code)
	}
	if env.postCount() != 1 {
		t.Errorf("重复 webhook 只应发送一次，实际 %d 条", env.postCount())
	}
}

func TestNumericObjectID(t *testing.T) {
	env := newFakeEnv(t)
	env.configure(t, 0)

	body := `{"objectType":"bug","objectID":3,"action":"edited","text":"no id here"}`
	rec := env.request(t, http.MethodPost, "application/json", body, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if env.postCount() != 1 {
		t.Errorf("数字 objectID 应能正常发送消息，实际 %d 条", env.postCount())
	}
}

func TestMissingBugIDReturns400(t *testing.T) {
	env := newFakeEnv(t)
	env.configure(t, 0)

	body := `{"objectType":"bug","objectID":"","action":"edited","text":"no id here"}`
	rec := env.request(t, http.MethodPost, "application/json", body, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, want 400", rec.Code)
	}
}

func TestBodyTooLargeReturns413(t *testing.T) {
	env := newFakeEnv(t)
	env.configure(t, 10)

	rec := env.request(t, http.MethodPost, "application/json", validBody(), nil)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d, want 413", rec.Code)
	}
}
