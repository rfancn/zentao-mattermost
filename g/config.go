package g

import (
	"strings"
	"time"

	"github.com/rfancn/zentao-mattermost/internal/errdef"
)

const (
	defaultListen          = ":8080"
	defaultPath            = "/webhook"
	defaultChannelPrefix   = "zentao-"
	defaultChannelType     = "P"
	defaultDedupeTTLSecond = 600
	defaultDedupeEntries   = 2048
	defaultHTTPTimeoutSec  = 10
	defaultMaxBodyBytes    = 1024 * 1024
	defaultReadTimeoutSec  = 10
	defaultShutdownSec     = 10
)

// 默认通知的禅道动作
var defaultNotifyActions = []string{"created", "edited", "activated", "resolved", "closed"}

// AppConfig 对应 TOML 根配置。
type AppConfig struct {
	Server     ServerConfig     `mapstructure:"server"`
	Zentao     ZentaoConfig     `mapstructure:"zentao"`
	Mattermost MattermostConfig `mapstructure:"mattermost"`
	Channel    ChannelConfig    `mapstructure:"channel"`
	Notify     NotifyConfig     `mapstructure:"notify"`
	Dedupe     DedupeConfig     `mapstructure:"dedupe"`
	HTTP       HTTPConfig       `mapstructure:"http"`
}

type ServerConfig struct {
	Listen                 string `mapstructure:"listen"`
	Path                   string `mapstructure:"path"`
	Token                  string `mapstructure:"token"`
	ReadTimeoutSeconds     int    `mapstructure:"read_timeout_seconds"`
	ShutdownTimeoutSeconds int    `mapstructure:"shutdown_timeout_seconds"`
}

type ZentaoConfig struct {
	URL      string `mapstructure:"url"`
	Account  string `mapstructure:"account"`
	Password string `mapstructure:"password"`
	Token    string `mapstructure:"token"`
}

type MattermostConfig struct {
	URL     string `mapstructure:"url"`
	Token   string `mapstructure:"token"`
	TeamID  string `mapstructure:"team_id"`
	BotUser string `mapstructure:"bot_user"`
}

type ChannelConfig struct {
	Prefix string `mapstructure:"prefix"`
	Type   string `mapstructure:"type"`
}

type NotifyConfig struct {
	Actions []string `mapstructure:"actions"`
}

type DedupeConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	TTLSeconds int  `mapstructure:"ttl_seconds"`
	MaxEntries int  `mapstructure:"max_entries"`
}

type HTTPConfig struct {
	TimeoutSeconds     int  `mapstructure:"timeout_seconds"`
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
	MaxBodyBytes       int  `mapstructure:"max_body_bytes"`
}

// Normalize 填充默认值并校验必填项。
func (c *AppConfig) Normalize() error {
	c.Zentao.URL = strings.TrimRight(c.Zentao.URL, "/")
	c.Mattermost.URL = strings.TrimRight(c.Mattermost.URL, "/")

	if c.Server.Token == "" {
		return errdef.ErrServerTokenRequired
	}
	if c.Zentao.URL == "" {
		return errdef.ErrZentaoURLRequired
	}
	if c.Zentao.Token == "" && (c.Zentao.Account == "" || c.Zentao.Password == "") {
		return errdef.ErrZentaoAuthRequired
	}
	if c.Mattermost.URL == "" {
		return errdef.ErrMattermostURLRequired
	}
	if c.Mattermost.Token == "" {
		return errdef.ErrMattermostTokenRequired
	}
	if c.Mattermost.TeamID == "" {
		return errdef.ErrMattermostTeamIDRequired
	}

	if c.Server.Listen == "" {
		c.Server.Listen = defaultListen
	}
	if c.Server.Path == "" {
		c.Server.Path = defaultPath
	}
	if !strings.HasPrefix(c.Server.Path, "/") {
		c.Server.Path = "/" + c.Server.Path
	}
	if c.Server.ReadTimeoutSeconds <= 0 {
		c.Server.ReadTimeoutSeconds = defaultReadTimeoutSec
	}
	if c.Server.ShutdownTimeoutSeconds <= 0 {
		c.Server.ShutdownTimeoutSeconds = defaultShutdownSec
	}

	if c.Channel.Prefix == "" {
		c.Channel.Prefix = defaultChannelPrefix
	}
	if c.Channel.Type == "" {
		c.Channel.Type = defaultChannelType
	}
	c.Channel.Type = strings.ToUpper(c.Channel.Type)
	if c.Channel.Type != "P" && c.Channel.Type != "O" {
		return errdef.ErrChannelTypeInvalid
	}

	if len(c.Notify.Actions) == 0 {
		c.Notify.Actions = append([]string(nil), defaultNotifyActions...)
	}

	if c.Dedupe.TTLSeconds <= 0 {
		c.Dedupe.TTLSeconds = defaultDedupeTTLSecond
	}
	if c.Dedupe.MaxEntries <= 0 {
		c.Dedupe.MaxEntries = defaultDedupeEntries
	}

	if c.HTTP.TimeoutSeconds <= 0 {
		c.HTTP.TimeoutSeconds = defaultHTTPTimeoutSec
	}
	if c.HTTP.MaxBodyBytes <= 0 {
		c.HTTP.MaxBodyBytes = defaultMaxBodyBytes
	}

	return nil
}

// DedupeTTL 返回去重记录的存活时间。
func (c *AppConfig) DedupeTTL() time.Duration {
	return time.Duration(c.Dedupe.TTLSeconds) * time.Second
}
