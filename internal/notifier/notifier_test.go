package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/g"
	"github.com/rfancn/zentao-mattermost/internal/errdef"
	"github.com/rfancn/zentao-mattermost/internal/zentao"
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

func TestExtractBugID(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"markdown link", "admin编辑了Bug [#3::test_bug3](http://x/index.php?m=bug&f=view&id=3)", "3"},
		{"url only", "http://x/index.php?m=bug&f=view&id=42", "42"},
		{"url with ampersand", "http://x/index.php?a=1&id=99", "99"},
		{"no id", "nothing here", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractBugID(tt.text); got != tt.want {
				t.Errorf("ExtractBugID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeChannelName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"basic", "zentao-TestUser", "zentao-testuser"},
		{"invalid chars", "zentao-张三 user!", "zentao-user"},
		{"trim leading punctuation", "--zentao-", "zentao"},
		{"too long", "zentao-" + strings.Repeat("a", 80), "zentao-" + strings.Repeat("a", 53)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeChannelName(tt.in); got != tt.want {
				t.Errorf("SanitizeChannelName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatMessage(t *testing.T) {
	hook := Webhook{
		Action: "edited",
		Actor:  "admin",
		Date:   "2026-10-08 17:43:58",
		Text:   "admin编辑了Bug",
	}
	bug := &zentao.Bug{
		ID:          3,
		Title:       "test_bug3",
		ProductName: "产品A",
		Severity:    2,
		Pri:         1,
		Status:      "active",
	}

	got := formatMessage(hook, bug, "testuser")
	for _, want := range []string{
		"@testuser",
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

type notifierEnv struct {
	bugStatus     int
	channelStatus int
	userStatus    int
	hasAssignee   bool

	mu    sync.Mutex
	posts []map[string]string

	ztSrv *httptest.Server
	mmSrv *httptest.Server
}

func newNotifierEnv(t *testing.T) *notifierEnv {
	t.Helper()

	e := &notifierEnv{
		bugStatus:     http.StatusOK,
		channelStatus: http.StatusOK,
		userStatus:    http.StatusOK,
		hasAssignee:   true,
	}

	ztMux := http.NewServeMux()
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
	mmMux.HandleFunc("/api/v4/teams/team1/channels/name/zentao-testuser", func(w http.ResponseWriter, r *http.Request) {
		if e.channelStatus != http.StatusOK {
			w.WriteHeader(e.channelStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "c1", "name": "zentao-testuser"})
	})
	mmMux.HandleFunc("/api/v4/channels/c1/members/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
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

func (e *notifierEnv) configure(t *testing.T) {
	t.Helper()

	g.Config = g.AppConfig{
		Server: g.ServerConfig{Token: "secret"},
		Zentao: g.ZentaoConfig{URL: e.ztSrv.URL, Token: "zt"},
		Mattermost: g.MattermostConfig{
			URL:     e.mmSrv.URL,
			Token:   "mm-token",
			TeamID:  "team1",
			BotUser: "zentao-bot",
		},
		Channel: g.ChannelConfig{Prefix: "zentao-", Type: "P"},
		HTTP:    g.HTTPConfig{TimeoutSeconds: 5},
	}
	if err := g.Config.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
}

func (e *notifierEnv) postCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.posts)
}

func testWebhook() Webhook {
	return Webhook{
		ObjectType: "bug",
		ObjectID:   "3",
		Action:     "edited",
		Actor:      "admin",
		Date:       "2026-10-08 17:43:58",
		Text:       "admin编辑了Bug [#3::test_bug3](http://x/index.php?m=bug&f=view&id=3)",
	}
}

func TestNotifySuccess(t *testing.T) {
	env := newNotifierEnv(t)

	if err := Notify(context.Background(), testWebhook()); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if env.postCount() != 1 {
		t.Fatalf("消息数量 = %d, want 1", env.postCount())
	}
}

func TestNotifyNoAssignee(t *testing.T) {
	env := newNotifierEnv(t)
	env.hasAssignee = false

	err := Notify(context.Background(), testWebhook())
	if !errors.Is(err, errdef.ErrNoAssignee) {
		t.Fatalf("Notify() error = %v, want ErrNoAssignee", err)
	}
	if env.postCount() != 0 {
		t.Errorf("无指派对象时不应发送消息")
	}
}

func TestNotifyZenTaoError(t *testing.T) {
	env := newNotifierEnv(t)
	env.bugStatus = http.StatusInternalServerError

	if err := Notify(context.Background(), testWebhook()); err == nil {
		t.Fatal("Notify() error = nil, want error")
	}
	if env.postCount() != 0 {
		t.Errorf("禅道失败时不应发送消息")
	}
}

func TestNotifyMattermostUserError(t *testing.T) {
	env := newNotifierEnv(t)
	env.userStatus = http.StatusNotFound

	if err := Notify(context.Background(), testWebhook()); err == nil {
		t.Fatal("Notify() error = nil, want error")
	}
	if env.postCount() != 0 {
		t.Errorf("Mattermost 用户查询失败时不应发送消息")
	}
}

func TestNotifyChannelError(t *testing.T) {
	env := newNotifierEnv(t)
	env.channelStatus = http.StatusInternalServerError

	if err := Notify(context.Background(), testWebhook()); err == nil {
		t.Fatal("Notify() error = nil, want error")
	}
	if env.postCount() != 0 {
		t.Errorf("频道查询失败时不应发送消息")
	}
}

func TestIDUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ID
		wantErr bool
	}{
		{name: "string", input: `"3"`, want: "3"},
		{name: "number", input: `3`, want: "3"},
		{name: "boolean", input: `true`, wantErr: true},
		{name: "null", input: `null`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var id ID
			err := json.Unmarshal([]byte(tt.input), &id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalJSON() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalJSON() error = %v", err)
			}
			if id != tt.want {
				t.Errorf("ID = %q, want %q", id, tt.want)
			}
		})
	}
}
