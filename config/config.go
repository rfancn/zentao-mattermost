package config

import (
	"strings"

	"github.com/rfancn/zentao-mattermost/internal/errdef"
)

// RootConfig 是应用程序的基础配置结构
type RootConfig struct {
	App AppConfig `mapstructure:"app"` // 应用特定配置
}

// AppConfig 对应 TOML 中的 [app] 配置。
type AppConfig struct {
	Listen string `mapstructure:"listen"`
	Token  string `mapstructure:"token"`

	Routes map[string]RouteConfig `mapstructure:"routes"`
}

// RouteConfig 定义一条 webhook 路由。
type RouteConfig struct {
	Source SourceConfig `mapstructure:"source"`
	Target TargetConfig `mapstructure:"target"`
}

// SourceConfig 定义 webhook 消息的解析来源。
type SourceConfig struct {
	Type     string `mapstructure:"type"`
	URL      string `mapstructure:"url"`
	Account  string `mapstructure:"account"`
	Password string `mapstructure:"password"`
}

// TargetConfig 定义通知的发送目标。
type TargetConfig struct {
	Type     string `mapstructure:"type"`
	URL      string `mapstructure:"url"`
	BotUser  string `mapstructure:"bot_user"`
	BotToken string `mapstructure:"bot_token"`
}

const (
	defaultListen = ":8080"
)

// Normalize 填充默认值并校验必填项。
func (c *AppConfig) Normalize() error {
	if c.Listen == "" {
		c.Listen = defaultListen
	}
	if c.Token == "" {
		return errdef.ErrServerTokenRequired
	}

	if len(c.Routes) == 0 {
		return errdef.ErrRoutesRequired
	}

	normalized := make(map[string]RouteConfig, len(c.Routes))
	for name, route := range c.Routes {
		// section name 即路由路径：去除首尾空白后作为规范名
		routeName := strings.TrimSpace(name)
		if routeName == "" {
			return errdef.ErrRouteNameRequired
		}
		if !validRouteName(routeName) {
			return errdef.ErrRouteNameInvalid
		}
		if _, exists := normalized[routeName]; exists {
			return errdef.ErrRouteNameDuplicated
		}

		if route.Source.Type == "" {
			return errdef.ErrSourceTypeRequired
		}
		route.Source.URL = strings.TrimRight(route.Source.URL, "/")

		if route.Target.Type == "" {
			return errdef.ErrTargetTypeRequired
		}
		route.Target.URL = strings.TrimRight(route.Target.URL, "/")

		normalized[routeName] = route
	}
	c.Routes = normalized

	return nil
}

// validRouteName 校验路由名可安全用作 URL 路径。
// 仅允许字母、数字、'-' 和 '_'。
func validRouteName(name string) bool {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
