package mattermost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rfancn/zentao-mattermost/g"
)

func setupConfig(t *testing.T, url, botUser string) {
	t.Helper()
	g.Config = g.AppConfig{
		Mattermost: g.MattermostConfig{
			URL:     url,
			Token:   "mm-token",
			TeamID:  "team1",
			BotUser: botUser,
		},
		Channel: g.ChannelConfig{Type: "P"},
		HTTP:    g.HTTPConfig{TimeoutSeconds: 5},
	}
	resetBotUser()
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

	setupConfig(t, srv.URL, "")

	u, err := GetUser(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if u.ID != "u1" {
		t.Fatalf("GetUser() = %+v", u)
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

	setupConfig(t, srv.URL, "zentao-bot")
	u, err := GetBotUser(context.Background())
	if err != nil || u.ID != "bot1" {
		t.Fatalf("GetBotUser() = %+v, err = %v", u, err)
	}

	setupConfig(t, srv.URL, "")
	u, err = GetBotUser(context.Background())
	if err != nil || u.ID != "me1" {
		t.Fatalf("GetBotUser() = %+v, err = %v", u, err)
	}
}

func TestGetOrCreateChannelExisting(t *testing.T) {
	var createCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/teams/team1/channels/name/zentao-testuser", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Channel{ID: "c1", Name: "zentao-testuser"})
	})
	mux.HandleFunc("/api/v4/channels", func(w http.ResponseWriter, r *http.Request) {
		createCalls.Add(1)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupConfig(t, srv.URL, "")

	c, err := GetOrCreateChannel(context.Background(), "zentao-testuser", "testuser")
	if err != nil || c.ID != "c1" {
		t.Fatalf("GetOrCreateChannel() = %+v, err = %v", c, err)
	}
	if createCalls.Load() != 0 {
		t.Error("频道已存在时不应创建")
	}
}

func TestGetOrCreateChannelCreates(t *testing.T) {
	var created atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/teams/team1/channels/name/zentao-testuser", func(w http.ResponseWriter, r *http.Request) {
		if !created.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(Channel{ID: "c1", Name: "zentao-testuser"})
	})
	mux.HandleFunc("/api/v4/channels", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["team_id"] != "team1" || payload["name"] != "zentao-testuser" ||
			payload["display_name"] != "ZenTao - testuser" || payload["type"] != "P" {
			t.Errorf("创建频道参数 = %+v", payload)
		}
		created.Store(true)
		_ = json.NewEncoder(w).Encode(Channel{ID: "c1", Name: "zentao-testuser"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupConfig(t, srv.URL, "")

	c, err := GetOrCreateChannel(context.Background(), "zentao-testuser", "testuser")
	if err != nil || c.ID != "c1" {
		t.Fatalf("GetOrCreateChannel() = %+v, err = %v", c, err)
	}
	if !created.Load() {
		t.Error("频道不存在时应创建")
	}
}

func TestEnsureMemberExistingAndAdd(t *testing.T) {
	var memberExists atomic.Bool
	var addCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/channels/c1/members/u1", func(w http.ResponseWriter, r *http.Request) {
		if !memberExists.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/api/v4/channels/c1/members", func(w http.ResponseWriter, r *http.Request) {
		addCalls.Add(1)
		memberExists.Store(true)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupConfig(t, srv.URL, "")

	memberExists.Store(true)
	if err := EnsureMember(context.Background(), "c1", "u1"); err != nil {
		t.Fatalf("已存在成员 EnsureMember() error = %v", err)
	}
	if addCalls.Load() != 0 {
		t.Error("成员已存在时不应添加")
	}

	memberExists.Store(false)
	if err := EnsureMember(context.Background(), "c1", "u1"); err != nil {
		t.Fatalf("添加成员 EnsureMember() error = %v", err)
	}
	if addCalls.Load() != 1 {
		t.Errorf("添加成员次数 = %d, want 1", addCalls.Load())
	}
}

func TestEnsureMemberConflictThenExists(t *testing.T) {
	var checkCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/channels/c1/members/u1", func(w http.ResponseWriter, r *http.Request) {
		if checkCalls.Add(1) == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/api/v4/channels/c1/members", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupConfig(t, srv.URL, "")

	if err := EnsureMember(context.Background(), "c1", "u1"); err != nil {
		t.Fatalf("EnsureMember() error = %v", err)
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

	setupConfig(t, srv.URL, "")

	if err := CreatePost(context.Background(), "c1", "hello"); err != nil {
		t.Fatalf("CreatePost() error = %v", err)
	}
	if got["channel_id"] != "c1" || got["message"] != "hello" {
		t.Fatalf("CreatePost() 请求体 = %+v", got)
	}
}
