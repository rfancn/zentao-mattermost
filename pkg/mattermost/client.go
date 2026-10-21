// Package mattermost 封装 Mattermost REST API。
package mattermost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/config"
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
	ID   string `json:"id"`
	Type string `json:"type"`
}

type post struct {
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
}

// Target 向 Mattermost 用户发送私聊消息。
type Target struct {
	baseURL string
	token   string
	botUser string

	botMu     sync.Mutex
	cachedBot *User

	dmMu    sync.Mutex
	directs map[string]string // userID -> direct channel ID
}

// NewTarget 创建 Mattermost Target。
func NewTarget(cfg config.TargetConfig) (*Target, error) {
	if cfg.URL == "" {
		return nil, errors.New("mattermost url 不能为空")
	}
	if cfg.BotToken == "" {
		return nil, errors.New("mattermost token 不能为空")
	}
	return &Target{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		token:   cfg.BotToken,
		botUser: cfg.BotUser,
		directs: make(map[string]string),
	}, nil
}

// Send 向通知对象发送私聊消息。
func (t *Target) Send(ctx context.Context, data map[string]string) error {
	recipient := data["recipient"]
	if recipient == "" {
		return errors.New("通知对象为空")
	}

	user, err := t.getUser(ctx, recipient)
	if err != nil {
		return err
	}

	bot, err := t.getBotUser(ctx)
	if err != nil {
		return err
	}

	channelID, err := t.getOrCreateDirectChannel(ctx, bot.ID, user.ID)
	if err != nil {
		return err
	}

	if err := t.createPost(ctx, channelID, data["text"]); err != nil {
		return err
	}

	sdk.Logger().Info("Mattermost 私聊消息已发送", "target", user.Username)
	return nil
}

// getUser 按用户名获取 Mattermost 用户。
func (t *Target) getUser(ctx context.Context, username string) (*User, error) {
	var u User
	endpoint := t.baseURL + "/api/v4/users/username/" + url.PathEscape(username)
	if err := t.do(ctx, http.MethodGet, endpoint, nil, &u); err != nil {
		return nil, fmt.Errorf("获取 Mattermost 用户 %q: %w", username, err)
	}
	return &u, nil
}

// getBotUser 获取机器人账号，优先使用配置的 bot_user，否则使用 token 所属用户。
func (t *Target) getBotUser(ctx context.Context) (*User, error) {
	t.botMu.Lock()
	if t.cachedBot != nil {
		u := *t.cachedBot
		t.botMu.Unlock()
		return &u, nil
	}
	t.botMu.Unlock()

	var u User
	var endpoint string
	if t.botUser != "" {
		endpoint = t.baseURL + "/api/v4/users/username/" + url.PathEscape(t.botUser)
	} else {
		endpoint = t.baseURL + "/api/v4/users/me"
	}

	if err := t.do(ctx, http.MethodGet, endpoint, nil, &u); err != nil {
		return nil, fmt.Errorf("获取 Mattermost 机器人账号: %w", err)
	}
	if u.ID == "" {
		return nil, errors.New("mattermost 机器人账号 ID 为空")
	}

	t.botMu.Lock()
	t.cachedBot = &u
	t.botMu.Unlock()

	return &u, nil
}

// getOrCreateDirectChannel 获取与指定用户的私聊频道，并按用户缓存。
func (t *Target) getOrCreateDirectChannel(ctx context.Context, botID, userID string) (string, error) {
	t.dmMu.Lock()
	if channelID, ok := t.directs[userID]; ok {
		t.dmMu.Unlock()
		return channelID, nil
	}
	t.dmMu.Unlock()

	var channel Channel
	if err := t.do(ctx, http.MethodPost, t.baseURL+"/api/v4/channels/direct", []string{botID, userID}, &channel); err != nil {
		return "", fmt.Errorf("创建 Mattermost 私聊频道: %w", err)
	}
	if channel.ID == "" {
		return "", errors.New("mattermost 私聊频道 ID 为空")
	}

	t.dmMu.Lock()
	t.directs[userID] = channel.ID
	t.dmMu.Unlock()

	return channel.ID, nil
}

// createPost 在频道中发送消息。
func (t *Target) createPost(ctx context.Context, channelID, message string) error {
	payload := post{ChannelID: channelID, Message: message}
	if err := t.do(ctx, http.MethodPost, t.baseURL+"/api/v4/posts", payload, nil); err != nil {
		return fmt.Errorf("发送 Mattermost 消息: %w", err)
	}
	return nil
}

func (t *Target) do(ctx context.Context, method, endpoint string, body any, result any) error {
	req := httpclient.New().R().
		SetContext(ctx).
		SetAuthToken(t.token)

	if body != nil {
		req.SetBody(body)
	}

	resp, err := req.Execute(method, endpoint)
	if err != nil {
		return fmt.Errorf("请求 Mattermost 接口: %w", err)
	}
	if resp.IsError() {
		return &httpclient.StatusError{StatusCode: resp.StatusCode(), Body: resp.String()}
	}

	// 不依赖响应 Content-Type，显式反序列化
	if result != nil && len(resp.Body()) > 0 {
		if err := json.Unmarshal(resp.Body(), result); err != nil {
			return fmt.Errorf("解析 Mattermost 响应 JSON: %w", err)
		}
	}
	return nil
}
