// Package notifier 负责把禅道 webhook 转换成 Mattermost 通知。
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/hdget/sdk"
	"github.com/rfancn/zentao-mattermost/g"
	"github.com/rfancn/zentao-mattermost/internal/errdef"
	"github.com/rfancn/zentao-mattermost/internal/mattermost"
	"github.com/rfancn/zentao-mattermost/internal/zentao"
)

var (
	bugIDRe    = regexp.MustCompile(`\[#([0-9]+)::`)
	urlBugIDRe = regexp.MustCompile(`[?&]id=([0-9]+)`)
)

// ID 兼容禅道 webhook 中 objectID 为字符串或数字两种情况。
type ID string

func (id *ID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*id = ID(s)
		return nil
	}

	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*id = ID(n.String())
	return nil
}

// Webhook 禅道 webhook 请求体。
type Webhook struct {
	ObjectType string `json:"objectType"`
	ObjectID   ID     `json:"objectID"`
	Action     string `json:"action"`
	Actor      string `json:"actor"`
	Date       string `json:"date"`
	Comment    string `json:"comment"`
	Text       string `json:"text"`
}

// Notify 执行完整的通知流程。
// 返回 errdef.ErrNoAssignee 时表示无需通知，调用方应视为成功跳过。
func Notify(ctx context.Context, hook Webhook) error {
	bugID := ExtractBugID(hook.Text)
	if bugID == "" && hook.ObjectID != "" {
		bugID = string(hook.ObjectID)
	}
	if bugID == "" {
		return errdef.ErrBugIDNotFound
	}

	bug, err := zentao.GetBug(ctx, bugID)
	if err != nil {
		return err
	}
	if bug.AssignedTo == nil || bug.AssignedTo.Account == "" {
		sdk.Logger().Debug("Bug 没有指派对象，跳过通知", "bugId", bug.ID)
		return errdef.ErrNoAssignee
	}

	target, err := mattermost.GetUser(ctx, bug.AssignedTo.Account)
	if err != nil {
		return err
	}

	channelName := SanitizeChannelName(g.Config.Channel.Prefix + target.Username)
	channel, err := mattermost.GetOrCreateChannel(ctx, channelName, target.Username)
	if err != nil {
		return err
	}

	bot, err := mattermost.GetBotUser(ctx)
	if err != nil {
		return err
	}

	if err := mattermost.EnsureMember(ctx, channel.ID, bot.ID); err != nil {
		return err
	}
	if target.ID != bot.ID {
		if err := mattermost.EnsureMember(ctx, channel.ID, target.ID); err != nil {
			return err
		}
	}

	message := formatMessage(hook, bug, target.Username)
	if err := mattermost.CreatePost(ctx, channel.ID, message); err != nil {
		return err
	}

	sdk.Logger().Info("Mattermost 通知已发送",
		"bugId", bug.ID,
		"channel", channelName,
		"target", target.Username,
		"action", hook.Action,
	)
	return nil
}

// ExtractBugID 从 webhook 文本中提取 Bug ID。
func ExtractBugID(text string) string {
	if m := bugIDRe.FindStringSubmatch(text); len(m) == 2 {
		return m[1]
	}
	if m := urlBugIDRe.FindStringSubmatch(text); len(m) == 2 {
		return m[1]
	}
	return ""
}

// SanitizeChannelName 生成符合 Mattermost 命名规则的频道名。
func SanitizeChannelName(s string) string {
	s = strings.ToLower(s)

	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}

	s = strings.Trim(b.String(), "-_.")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

func formatMessage(h Webhook, b *zentao.Bug, username string) string {
	return fmt.Sprintf("@%s\n\n**禅道 Bug 通知**\n\n- Bug #%d：%s\n- 产品：%s\n- 严重程度：%d\n- 优先级：%d\n- 状态：%s\n- 操作人：%s\n- 操作：%s\n- 时间：%s\n\n%s",
		username, b.ID, b.Title, b.ProductName, b.Severity, b.Pri, b.Status, h.Actor, h.Action, h.Date, h.Text)
}
