// Package zentao 封装禅道 REST API 与 webhook 解析。
package zentao

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

// Source 解析禅道 webhook 并查询 Bug 详情。
type Source struct {
	baseURL  string
	account  string
	password string

	tokenMu sync.Mutex
	token   string
}

// NewSource 创建禅道 Source。
func NewSource(cfg config.SourceConfig) (*Source, error) {
	if cfg.URL == "" {
		return nil, errors.New("zentao url 不能为空")
	}
	if cfg.Account == "" || cfg.Password == "" {
		return nil, errors.New("zentao account 和 password 不能为空")
	}
	return &Source{
		baseURL:  strings.TrimRight(cfg.URL, "/"),
		account:  cfg.Account,
		password: cfg.Password,
	}, nil
}

// Parse 解析 webhook 请求体并生成通知消息。
// 非 Bug、未订阅动作或无指派对象时返回 (nil, nil)。
func (s *Source) Parse(ctx context.Context, body []byte) (map[string]string, error) {
	hook, err := ParseWebhook(body)
	if err != nil {
		sdk.Logger().Error("解析禅道 webhook 失败", "err", err)
		return nil, fmt.Errorf("%w: %v", errdef.ErrInvalidWebhook, err)
	}

	if hook.ObjectType != "bug" {
		return nil, nil
	}

	bugID := hook.BugID()
	if bugID == "" {
		sdk.Logger().Error("无法从 webhook 中提取 Bug ID", "text", hook.Text)
		return nil, errdef.ErrBugIDNotFound
	}

	bug, err := s.getBug(ctx, bugID)
	if err != nil {
		return nil, err
	}
	if bug.AssignedTo == nil || bug.AssignedTo.Account == "" {
		sdk.Logger().Debug("Bug 没有指派对象，跳过通知", "bugId", bug.ID)
		return nil, nil
	}

	return map[string]string{
		"recipient": bug.AssignedTo.Account,
		"text":      formatMessage(hook, bug),
	}, nil
}

func (s *Source) getBug(ctx context.Context, id string) (*Bug, error) {
	tok, err := s.getToken(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := s.baseURL + "/api.php/v1/bugs/" + url.PathEscape(id)
	bug, err := s.fetchBug(ctx, endpoint, tok)
	if err != nil && httpclient.IsStatus(err, http.StatusUnauthorized) {
		s.invalidateToken()
		tok, tokenErr := s.getToken(ctx)
		if tokenErr != nil {
			return nil, fmt.Errorf("刷新禅道 token: %w", tokenErr)
		}
		bug, err = s.fetchBug(ctx, endpoint, tok)
	}
	if err != nil {
		return nil, fmt.Errorf("获取禅道 Bug %s: %w", id, err)
	}
	return bug, nil
}

func (s *Source) fetchBug(ctx context.Context, endpoint, token string) (*Bug, error) {
	resp, err := httpclient.New().R().
		SetContext(ctx).
		SetHeader("BotToken", token).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("请求禅道 Bug 接口: %w", err)
	}
	if resp.IsError() {
		return nil, &httpclient.StatusError{StatusCode: resp.StatusCode(), Body: resp.String()}
	}

	var bug Bug
	if len(resp.Body()) > 0 {
		if err := json.Unmarshal(resp.Body(), &bug); err != nil {
			return nil, fmt.Errorf("解析禅道 Bug 响应 JSON: %w", err)
		}
	}
	return &bug, nil
}

func (s *Source) getToken(ctx context.Context) (string, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()

	if s.token != "" {
		return s.token, nil
	}

	return s.refreshTokenLocked(ctx)
}

func (s *Source) refreshTokenLocked(ctx context.Context) (string, error) {
	resp, err := httpclient.New().R().
		SetContext(ctx).
		SetBody(map[string]string{
			"account":  s.account,
			"password": s.password,
		}).
		Post(s.baseURL + "/api.php/v1/tokens")
	if err != nil {
		return "", fmt.Errorf("请求禅道 token 接口: %w", err)
	}
	if resp.IsError() {
		return "", fmt.Errorf("获取禅道 token: %w", &httpclient.StatusError{StatusCode: resp.StatusCode(), Body: resp.String()})
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

	s.token = out.Token
	sdk.Logger().Debug("获取禅道 token 成功")
	return s.token, nil
}

func (s *Source) invalidateToken() {
	s.tokenMu.Lock()
	s.token = ""
	s.tokenMu.Unlock()
}

func formatMessage(h Webhook, b *Bug) string {
	return fmt.Sprintf("**禅道 Bug 通知**\n\n- Bug #%d：%s\n- 产品：%s\n- 严重程度：%d\n- 优先级：%d\n- 状态：%s\n- 操作人：%s\n- 操作：%s\n- 时间：%s\n\n%s",
		b.ID, b.Title, b.ProductName, b.Severity, b.Pri, b.Status, h.Actor, h.Action, h.Date, h.Text)
}
