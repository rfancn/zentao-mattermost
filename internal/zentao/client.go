// Package zentao 封装禅道 REST API。
package zentao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/g"
	"github.com/rfancn/zentao-mattermost/internal/errdef"
	"github.com/rfancn/zentao-mattermost/internal/httpclient"
)

// User 禅道用户信息。
type User struct {
	ID       int    `json:"id"`
	Account  string `json:"account"`
	RealName string `json:"realname"`
	Avatar   string `json:"avatar"`
}

// Bug 禅道 Bug 信息。
type Bug struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Severity    int    `json:"severity"`
	Pri         int    `json:"pri"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	ProductName string `json:"productName"`
	AssignedTo  *User  `json:"assignedTo"`
}

var (
	tokenMu sync.Mutex
	token   string
)

// GetBug 获取指定 Bug，token 失效时自动刷新一次。
func GetBug(ctx context.Context, id string) (*Bug, error) {
	tok, err := getToken(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := g.Config.Zentao.URL + "/api.php/v1/bugs/" + url.PathEscape(id)
	bug, err := fetchBug(ctx, endpoint, tok)
	if err != nil && errdef.IsStatus(err, http.StatusUnauthorized) && g.Config.Zentao.Token == "" {
		invalidateToken()
		tok, tokenErr := getToken(ctx)
		if tokenErr != nil {
			return nil, fmt.Errorf("刷新禅道 token: %w", tokenErr)
		}
		bug, err = fetchBug(ctx, endpoint, tok)
	}
	if err != nil {
		return nil, fmt.Errorf("获取禅道 Bug %s: %w", id, err)
	}
	return bug, nil
}

func fetchBug(ctx context.Context, endpoint, token string) (*Bug, error) {
	resp, err := httpclient.New().R().
		SetContext(ctx).
		SetHeader("Token", token).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("请求禅道 Bug 接口: %w", err)
	}
	if resp.IsError() {
		return nil, &errdef.StatusError{StatusCode: resp.StatusCode(), Body: resp.String()}
	}

	var bug Bug
	if len(resp.Body()) > 0 {
		if err := json.Unmarshal(resp.Body(), &bug); err != nil {
			return nil, fmt.Errorf("解析禅道 Bug 响应 JSON: %w", err)
		}
	}
	return &bug, nil
}

func getToken(ctx context.Context) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if token != "" {
		return token, nil
	}

	if g.Config.Zentao.Token != "" {
		token = g.Config.Zentao.Token
		return token, nil
	}

	return refreshTokenLocked(ctx)
}

func refreshTokenLocked(ctx context.Context) (string, error) {
	resp, err := httpclient.New().R().
		SetContext(ctx).
		SetBody(map[string]string{
			"account":  g.Config.Zentao.Account,
			"password": g.Config.Zentao.Password,
		}).
		Post(g.Config.Zentao.URL + "/api.php/v1/tokens")
	if err != nil {
		return "", fmt.Errorf("请求禅道 token 接口: %w", err)
	}
	if resp.IsError() {
		return "", fmt.Errorf("获取禅道 token: %w", &errdef.StatusError{StatusCode: resp.StatusCode(), Body: resp.String()})
	}

	var out struct {
		Token string `json:"token"`
	}
	if len(resp.Body()) > 0 {
		if err := json.Unmarshal(resp.Body(), &out); err != nil {
			return "", fmt.Errorf("解析禅道 token 响应 JSON: %w", err)
		}
	}
	if out.Token == "" {
		return "", errors.New("禅道 token 响应为空")
	}

	token = out.Token
	sdk.Logger().Debug("获取禅道 token 成功")
	return token, nil
}

func invalidateToken() {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if g.Config.Zentao.Token == "" {
		token = ""
	}
}

// resetToken 仅用于测试，清空进程内缓存的 token。
func resetToken() {
	tokenMu.Lock()
	token = ""
	tokenMu.Unlock()
}
