package zentao

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/g"
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

func setupConfig(t *testing.T, url, token, account, password string) {
	t.Helper()
	g.Config = g.AppConfig{
		Zentao: g.ZentaoConfig{URL: url, Token: token, Account: account, Password: password},
		HTTP:   g.HTTPConfig{TimeoutSeconds: 5},
	}
	resetToken()
}

func TestGetBugWithStaticToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.php/v1/bugs/3" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Token") != "static-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(Bug{
			ID:         3,
			Title:      "test_bug3",
			AssignedTo: &User{ID: 2, Account: "testuser"},
		})
	}))
	defer srv.Close()

	setupConfig(t, srv.URL, "static-token", "", "")

	bug, err := GetBug(context.Background(), "3")
	if err != nil {
		t.Fatalf("GetBug() error = %v", err)
	}
	if bug.ID != 3 || bug.AssignedTo == nil || bug.AssignedTo.Account != "testuser" {
		t.Fatalf("GetBug() = %+v", bug)
	}
}

func TestGetBugRefreshesTokenOnUnauthorized(t *testing.T) {
	var tokenCalls, bugCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api.php/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		call := tokenCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": fmt.Sprintf("token-%d", call)})
	})
	mux.HandleFunc("/api.php/v1/bugs/3", func(w http.ResponseWriter, r *http.Request) {
		bugCalls.Add(1)
		if r.Header.Get("Token") != "token-2" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(Bug{ID: 3, Title: "ok"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupConfig(t, srv.URL, "", "admin", "password")

	bug, err := GetBug(context.Background(), "3")
	if err != nil {
		t.Fatalf("GetBug() error = %v", err)
	}
	if bug.Title != "ok" {
		t.Fatalf("GetBug() = %+v", bug)
	}
	if got := tokenCalls.Load(); got != 2 {
		t.Errorf("token 请求次数 = %d, want 2", got)
	}
	if got := bugCalls.Load(); got != 2 {
		t.Errorf("Bug 请求次数 = %d, want 2", got)
	}
}

func TestGetBugStaticTokenUnauthorizedDoesNotRefresh(t *testing.T) {
	var tokenCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api.php/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/api.php/v1/bugs/3", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupConfig(t, srv.URL, "static-token", "", "")

	if _, err := GetBug(context.Background(), "3"); err == nil {
		t.Fatal("GetBug() error = nil, want error")
	}
	if got := tokenCalls.Load(); got != 0 {
		t.Errorf("静态 token 模式下不应请求 token 接口，实际 %d 次", got)
	}
}

func TestRefreshTokenEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	setupConfig(t, srv.URL, "", "admin", "password")

	if _, err := GetBug(context.Background(), "3"); err == nil {
		t.Fatal("GetBug() error = nil, want error")
	}
}
