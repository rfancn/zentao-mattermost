// Package notifier 定义 webhook 消息的解析与通知抽象，并负责 source/target 的组合。
package notifier

import (
	"context"
	"fmt"

	"github.com/rfancn/zentao-mattermost/config"
	"github.com/rfancn/zentao-mattermost/internal/errdef"
	"github.com/rfancn/zentao-mattermost/pkg/mattermost"
	"github.com/rfancn/zentao-mattermost/pkg/zentao"
)

// Notifier 处理通过安全校验的 webhook 请求体。
type Notifier interface {
	Notify(ctx context.Context, body []byte) error
}

// Factory 创建 Notifier 实例。
type Factory func() Notifier

// Source 解析 webhook 请求体并生成待发送的数据。
// 返回 (nil, nil) 表示该消息无需通知。
type Source interface {
	Parse(ctx context.Context, body []byte) (map[string]string, error)
}

// Target 发送 Source 解析后的数据，并自行决定读取哪些字段。
type Target interface {
	Send(ctx context.Context, data map[string]string) error
}

// sourceFactories 定义 source.type 到 Source 构造函数的映射。
var sourceFactories = map[string]func(config.SourceConfig) (Source, error){
	"zentao": func(cfg config.SourceConfig) (Source, error) {
		return zentao.NewSource(cfg)
	},
}

// targetFactories 定义 target.type 到 Target 构造函数的映射。
var targetFactories = map[string]func(config.TargetConfig) (Target, error){
	"mattermost": func(cfg config.TargetConfig) (Target, error) {
		return mattermost.NewTarget(cfg)
	},
}

// New 根据 source/target 类型创建 Notifier。
func New(sourceCfg config.SourceConfig, targetCfg config.TargetConfig) (Notifier, error) {
	srcFactory, ok := sourceFactories[sourceCfg.Type]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errdef.ErrSourceTypeUnknown, sourceCfg.Type)
	}
	src, err := srcFactory(sourceCfg)
	if err != nil {
		return nil, err
	}

	tgtFactory, ok := targetFactories[targetCfg.Type]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errdef.ErrTargetTypeUnknown, targetCfg.Type)
	}
	tgt, err := tgtFactory(targetCfg)
	if err != nil {
		return nil, err
	}

	return &notifierImpl{source: src, target: tgt}, nil
}

type notifierImpl struct {
	source Source
	target Target
}

// Notify 解析请求体并发送通知。
func (n *notifierImpl) Notify(ctx context.Context, body []byte) error {
	data, err := n.source.Parse(ctx, body)
	if err != nil || data == nil {
		return err
	}
	return n.target.Send(ctx, data)
}
