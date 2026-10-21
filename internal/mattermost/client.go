// Package mattermost 封装 Mattermost REST API。
package mattermost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/rfancn/zentao-mattermost/g"
	"github.com/rfancn/zentao-mattermost/internal/errdef"
	"github.com/rfancn/zentao-mattermost/internal/httpclient"
)

// User Mattermost 用户信息。
type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// Channel Mattermost 频道信息。
type Channel struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type post struct {
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
}

var (
	botMu   sync.Mutex
	botUser *User
)

// GetUser 按用户名获取 Mattermost 用户。
func GetUser(ctx context.Context, username string) (*User, error) {
	var u User
	endpoint := g.Config.Mattermost.URL + "/api/v4/users/username/" + url.PathEscape(username)
	if err := do(ctx, http.MethodGet, endpoint, nil, &u); err != nil {
		return nil, fmt.Errorf("获取 Mattermost 用户 %q: %w", username, err)
	}
	return &u, nil
}

// GetBotUser 获取机器人账号，优先使用配置的 bot_user，否则使用 token 所属用户。
func GetBotUser(ctx context.Context) (*User, error) {
	botMu.Lock()
	if botUser != nil {
		u := *botUser
		botMu.Unlock()
		return &u, nil
	}
	botMu.Unlock()

	var u User
	var endpoint string
	if g.Config.Mattermost.BotUser != "" {
		endpoint = g.Config.Mattermost.URL + "/api/v4/users/username/" + url.PathEscape(g.Config.Mattermost.BotUser)
	} else {
		endpoint = g.Config.Mattermost.URL + "/api/v4/users/me"
	}

	if err := do(ctx, http.MethodGet, endpoint, nil, &u); err != nil {
		return nil, fmt.Errorf("获取 Mattermost 机器人账号: %w", err)
	}
	if u.ID == "" {
		return nil, errors.New("mattermost 机器人账号 ID 为空")
	}

	botMu.Lock()
	botUser = &u
	botMu.Unlock()

	return &u, nil
}

// GetOrCreateChannel 按名称获取频道，不存在时创建。
func GetOrCreateChannel(ctx context.Context, name, username string) (*Channel, error) {
	endpoint := g.Config.Mattermost.URL + "/api/v4/teams/" + url.PathEscape(g.Config.Mattermost.TeamID) +
		"/channels/name/" + url.PathEscape(name)

	var existing Channel
	err := do(ctx, http.MethodGet, endpoint, nil, &existing)
	if err == nil {
		return &existing, nil
	}
	if !errdef.IsStatus(err, http.StatusNotFound) {
		return nil, fmt.Errorf("查找 Mattermost 频道 %q: %w", name, err)
	}

	var created Channel
	payload := map[string]string{
		"team_id":      g.Config.Mattermost.TeamID,
		"name":         name,
		"display_name": "ZenTao - " + username,
		"type":         g.Config.Channel.Type,
	}
	if createErr := do(ctx, http.MethodPost, g.Config.Mattermost.URL+"/api/v4/channels", payload, &created); createErr == nil {
		return &created, nil
	} else {
		// 并发创建时另一个请求可能已经成功，重新读取一次
		var reread Channel
		if readErr := do(ctx, http.MethodGet, endpoint, nil, &reread); readErr == nil {
			return &reread, nil
		}
		return nil, fmt.Errorf("创建 Mattermost 频道 %q: %w", name, createErr)
	}
}

// EnsureMember 确保用户是频道成员。
func EnsureMember(ctx context.Context, channelID, userID string) error {
	checkEndpoint := g.Config.Mattermost.URL + "/api/v4/channels/" + url.PathEscape(channelID) +
		"/members/" + url.PathEscape(userID)

	var member map[string]any
	err := do(ctx, http.MethodGet, checkEndpoint, nil, &member)
	if err == nil {
		return nil
	}
	if !errdef.IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("检查 Mattermost 频道成员 %s: %w", userID, err)
	}

	var added map[string]any
	addEndpoint := g.Config.Mattermost.URL + "/api/v4/channels/" + url.PathEscape(channelID) + "/members"
	if addErr := do(ctx, http.MethodPost, addEndpoint, map[string]string{"user_id": userID}, &added); addErr != nil {
		// 并发添加或成员已存在时重新检查一次
		if errdef.IsStatus(addErr, http.StatusBadRequest) || errdef.IsStatus(addErr, http.StatusConflict) {
			var recheck map[string]any
			if checkErr := do(ctx, http.MethodGet, checkEndpoint, nil, &recheck); checkErr == nil {
				return nil
			}
		}
		return fmt.Errorf("添加 Mattermost 频道成员 %s: %w", userID, addErr)
	}
	return nil
}

// CreatePost 在频道中发送消息。
func CreatePost(ctx context.Context, channelID, message string) error {
	payload := post{ChannelID: channelID, Message: message}
	if err := do(ctx, http.MethodPost, g.Config.Mattermost.URL+"/api/v4/posts", payload, nil); err != nil {
		return fmt.Errorf("发送 Mattermost 消息: %w", err)
	}
	return nil
}

func do(ctx context.Context, method, endpoint string, body any, result any) error {
	req := httpclient.New().R().
		SetContext(ctx).
		SetAuthToken(g.Config.Mattermost.Token)

	if body != nil {
		req.SetBody(body)
	}

	resp, err := req.Execute(method, endpoint)
	if err != nil {
		return fmt.Errorf("请求 Mattermost 接口: %w", err)
	}
	if resp.IsError() {
		return &errdef.StatusError{StatusCode: resp.StatusCode(), Body: resp.String()}
	}

	// 不依赖响应 Content-Type，显式反序列化
	if result != nil && len(resp.Body()) > 0 {
		if err := json.Unmarshal(resp.Body(), result); err != nil {
			return fmt.Errorf("解析 Mattermost 响应 JSON: %w", err)
		}
	}
	return nil
}

// resetBotUser 仅用于测试，清空进程内缓存的机器人账号。
func resetBotUser() {
	botMu.Lock()
	botUser = nil
	botMu.Unlock()
}
