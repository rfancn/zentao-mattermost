package mattermost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rfancn/zentao-mattermost/config"
)

func newTarget(t *testing.T, url, botUser string) *Target {
	t.Helper()

	tgt, err := NewTarget(config.TargetConfig{
		URL:      url,
		BotToken: "mm-token",
		BotUser:  botUser,
	})
	if err != nil {
		t.Fatalf("NewTarget() error = %v", err)
	}
	return tgt
}

func TestNewTargetValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.TargetConfig
	}{
		{name: "missing url", cfg: config.TargetConfig{BotToken: "mm"}},
		{name: "missing token", cfg: config.TargetConfig{URL: "http://mattermost"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewTarget(tt.cfg); err == nil {
				t.Fatal("NewTarget() error = nil, want error")
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mm-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(User{ID: "u1", Username: "testuser"})
	}))
	defer srv.Close()

	u, err := newTarget(t, srv.URL, "").getUser(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("getUser() error = %v", err)
	}
	if u.ID != "u1" {
		t.Fatalf("getUser() = %+v", u)
	}
}

func TestGetBotUserByUsernameAndMe(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/users/username/zentao-bot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(User{ID: "bot1", Username: "zentao-bot"})
	})
	mux.HandleFunc("/api/v4/users/me", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(User{ID: "me1", Username: "me"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, err := newTarget(t, srv.URL, "zentao-bot").getBotUser(context.Background())
	if err != nil || u.ID != "bot1" {
		t.Fatalf("getBotUser() = %+v, err = %v", u, err)
	}

	u, err = newTarget(t, srv.URL, "").getBotUser(context.Background())
	if err != nil || u.ID != "me1" {
		t.Fatalf("getBotUser() = %+v, err = %v", u, err)
	}
}

func TestGetOrCreateDirectChannel(t *testing.T) {
	var gotIDs []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/channels/direct" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotIDs)
		_ = json.NewEncoder(w).Encode(Channel{ID: "c1", Type: "D"})
	}))
	defer srv.Close()

	id, err := newTarget(t, srv.URL, "").getOrCreateDirectChannel(context.Background(), "bot1", "u1")
	if err != nil {
		t.Fatalf("getOrCreateDirectChannel() error = %v", err)
	}
	if id != "c1" {
		t.Fatalf("channelID = %q, want c1", id)
	}
	if len(gotIDs) != 2 || gotIDs[0] != "bot1" || gotIDs[1] != "u1" {
		t.Fatalf("请求用户 = %v, want [bot1 u1]", gotIDs)
	}
}

func TestDirectChannelIsCached(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(Channel{ID: "c1", Type: "D"})
	}))
	defer srv.Close()

	tgt := newTarget(t, srv.URL, "")
	for i := 0; i < 2; i++ {
		id, err := tgt.getOrCreateDirectChannel(context.Background(), "bot1", "u1")
		if err != nil {
			t.Fatalf("第 %d 次 getOrCreateDirectChannel() error = %v", i+1, err)
		}
		if id != "c1" {
			t.Fatalf("channelID = %q, want c1", id)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("创建私聊频道请求次数 = %d, want 1（应命中缓存）", got)
	}
}

func TestCreatePost(t *testing.T) {
	var got map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/posts" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p1"}`))
	}))
	defer srv.Close()

	if err := newTarget(t, srv.URL, "").createPost(context.Background(), "c1", "hello"); err != nil {
		t.Fatalf("createPost() error = %v", err)
	}
	if got["channel_id"] != "c1" || got["message"] != "hello" {
		t.Fatalf("createPost() 请求体 = %+v", got)
	}
}
