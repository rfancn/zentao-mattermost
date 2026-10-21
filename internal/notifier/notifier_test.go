package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/config"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "notifier-test")
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
	if err := sdk.New("notifier-test", sdk.WithConfigContent([]byte(content))).Initialize(); err != nil {
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type env struct {
	bugStatus     int
	userStatus    int
	channelStatus int
	hasAssignee   bool

	mu    sync.Mutex
	posts []map[string]string

	ztSrv *httptest.Server
	mmSrv *httptest.Server
}

func newEnv(t *testing.T) *env {
	t.Helper()

	e := &env{
		bugStatus:     http.StatusOK,
		userStatus:    http.StatusOK,
		channelStatus: http.StatusOK,
		hasAssignee:   true,
	}

	ztMux := http.NewServeMux()
	ztMux.HandleFunc("/api.php/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "zt"})
	})
	ztMux.HandleFunc("/api.php/v1/bugs/3", func(w http.ResponseWriter, r *http.Request) {
		if e.bugStatus != http.StatusOK {
			w.WriteHeader(e.bugStatus)
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
		if e.hasAssignee {
			bug["assignedTo"] = map[string]any{"id": 2, "account": "testuser"}
		}
		_ = json.NewEncoder(w).Encode(bug)
	})
	e.ztSrv = httptest.NewServer(ztMux)

	mmMux := http.NewServeMux()
	mmMux.HandleFunc("/api/v4/users/username/testuser", func(w http.ResponseWriter, r *http.Request) {
		if e.userStatus != http.StatusOK {
			w.WriteHeader(e.userStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "u1", "username": "testuser"})
	})
	mmMux.HandleFunc("/api/v4/users/username/zentao-bot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "bot1", "username": "zentao-bot"})
	})
	mmMux.HandleFunc("/api/v4/channels/direct", func(w http.ResponseWriter, r *http.Request) {
		if e.channelStatus != http.StatusOK {
			w.WriteHeader(e.channelStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "c1", "type": "D"})
	})
	mmMux.HandleFunc("/api/v4/posts", func(w http.ResponseWriter, r *http.Request) {
		var p map[string]string
		_ = json.NewDecoder(r.Body).Decode(&p)
		e.mu.Lock()
		e.posts = append(e.posts, p)
		e.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p1"}`))
	})
	e.mmSrv = httptest.NewServer(mmMux)

	t.Cleanup(func() {
		e.ztSrv.Close()
		e.mmSrv.Close()
	})

	e.configure(t)
	return e
}

func (e *env) configure(t *testing.T) {
	t.Helper()

}

func (e *env) notifier(t *testing.T) interface {
	Notify(ctx context.Context, body []byte) error
} {
	t.Helper()

	n, err := New(
		config.SourceConfig{Type: "zentao", URL: e.ztSrv.URL, Account: "admin", Password: "password"},
		config.TargetConfig{Type: "mattermost", URL: e.mmSrv.URL, BotToken: "mm-token", BotUser: "zentao-bot"},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return n
}

func (e *env) postCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.posts)
}

func (e *env) lastPost() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.posts) == 0 {
		return nil
	}
	return e.posts[len(e.posts)-1]
}

func testBody() []byte {
	return []byte(`{"objectType":"bug","objectID":"3","action":"edited","actor":"admin","date":"2026-10-08 17:43:58","text":"admin编辑了Bug [#3::test_bug3](http://x/index.php?m=bug&f=view&id=3)"}`)
}

func TestNotifySuccess(t *testing.T) {
	e := newEnv(t)

	if err := e.notifier(t).Notify(context.Background(), testBody()); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if e.postCount() != 1 {
		t.Fatalf("消息数量 = %d, want 1", e.postCount())
	}

	p := e.lastPost()
	for _, want := range []string{"Bug #3：test_bug3", "产品：产品A"} {
		if !strings.Contains(p["message"], want) {
			t.Errorf("消息缺少 %q:\n%s", want, p["message"])
		}
	}
}

func TestNotifyNoAssigneeSkips(t *testing.T) {
	e := newEnv(t)
	e.hasAssignee = false

	if err := e.notifier(t).Notify(context.Background(), testBody()); err != nil {
		t.Fatalf("Notify() error = %v, want nil", err)
	}
	if e.postCount() != 0 {
		t.Errorf("无指派对象时不应发送消息")
	}
}

func TestNotifyZenTaoError(t *testing.T) {
	e := newEnv(t)
	e.bugStatus = http.StatusInternalServerError

	if err := e.notifier(t).Notify(context.Background(), testBody()); err == nil {
		t.Fatal("Notify() error = nil, want error")
	}
	if e.postCount() != 0 {
		t.Errorf("禅道失败时不应发送消息")
	}
}

func TestNotifyMattermostUserError(t *testing.T) {
	e := newEnv(t)
	e.userStatus = http.StatusNotFound

	if err := e.notifier(t).Notify(context.Background(), testBody()); err == nil {
		t.Fatal("Notify() error = nil, want error")
	}
	if e.postCount() != 0 {
		t.Errorf("Mattermost 用户查询失败时不应发送消息")
	}
}

func TestNotifyDirectChannelError(t *testing.T) {
	e := newEnv(t)
	e.channelStatus = http.StatusInternalServerError

	if err := e.notifier(t).Notify(context.Background(), testBody()); err == nil {
		t.Fatal("Notify() error = nil, want error")
	}
	if e.postCount() != 0 {
		t.Errorf("频道查询失败时不应发送消息")
	}
}
