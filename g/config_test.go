package g

import (
	"errors"
	"testing"

	"github.com/rfancn/zentao-mattermost/internal/errdef"
)

func validConfig() AppConfig {
	return AppConfig{
		Server: ServerConfig{Token: "secret"},
		Zentao: ZentaoConfig{URL: "http://zentao", Token: "zt"},
		Mattermost: MattermostConfig{
			URL:    "http://mattermost",
			Token:  "mm",
			TeamID: "team",
		},
	}
}

func TestNormalizeDefaults(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if cfg.Server.Listen != ":8080" {
		t.Errorf("Server.Listen = %q", cfg.Server.Listen)
	}
	if cfg.Server.Path != "/webhook" {
		t.Errorf("Server.Path = %q", cfg.Server.Path)
	}
	if cfg.Channel.Prefix != "zentao-" {
		t.Errorf("Channel.Prefix = %q", cfg.Channel.Prefix)
	}
	if cfg.Channel.Type != "P" {
		t.Errorf("Channel.Type = %q", cfg.Channel.Type)
	}
	if len(cfg.Notify.Actions) != 5 {
		t.Errorf("Notify.Actions = %v", cfg.Notify.Actions)
	}
	if cfg.Dedupe.TTLSeconds != 600 || cfg.Dedupe.MaxEntries != 2048 {
		t.Errorf("Dedupe = %+v", cfg.Dedupe)
	}
	if cfg.HTTP.TimeoutSeconds != 10 || cfg.HTTP.MaxBodyBytes != 1024*1024 {
		t.Errorf("HTTP = %+v", cfg.HTTP)
	}
	if cfg.Server.ReadTimeoutSeconds != 10 || cfg.Server.ShutdownTimeoutSeconds != 10 {
		t.Errorf("Server timeouts = %+v", cfg.Server)
	}
}

func TestNormalizeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*AppConfig)
		wantErr error
	}{
		{
			name:    "missing server token",
			mutate:  func(c *AppConfig) { c.Server.Token = "" },
			wantErr: errdef.ErrServerTokenRequired,
		},
		{
			name:    "missing zentao url",
			mutate:  func(c *AppConfig) { c.Zentao.URL = "" },
			wantErr: errdef.ErrZentaoURLRequired,
		},
		{
			name: "missing zentao auth",
			mutate: func(c *AppConfig) {
				c.Zentao.Token = ""
				c.Zentao.Account = ""
				c.Zentao.Password = ""
			},
			wantErr: errdef.ErrZentaoAuthRequired,
		},
		{
			name:    "missing mattermost url",
			mutate:  func(c *AppConfig) { c.Mattermost.URL = "" },
			wantErr: errdef.ErrMattermostURLRequired,
		},
		{
			name:    "missing mattermost token",
			mutate:  func(c *AppConfig) { c.Mattermost.Token = "" },
			wantErr: errdef.ErrMattermostTokenRequired,
		},
		{
			name:    "missing mattermost team id",
			mutate:  func(c *AppConfig) { c.Mattermost.TeamID = "" },
			wantErr: errdef.ErrMattermostTeamIDRequired,
		},
		{
			name:    "invalid channel type",
			mutate:  func(c *AppConfig) { c.Channel.Type = "X" },
			wantErr: errdef.ErrChannelTypeInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			err := cfg.Normalize()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Normalize() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeTrailingSlashAndChannelType(t *testing.T) {
	cfg := validConfig()
	cfg.Zentao.URL = "http://zentao/"
	cfg.Mattermost.URL = "http://mattermost/"
	cfg.Channel.Type = "o"
	cfg.Server.Path = "hooks/zentao"

	if err := cfg.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if cfg.Zentao.URL != "http://zentao" {
		t.Errorf("Zentao.URL = %q", cfg.Zentao.URL)
	}
	if cfg.Mattermost.URL != "http://mattermost" {
		t.Errorf("Mattermost.URL = %q", cfg.Mattermost.URL)
	}
	if cfg.Channel.Type != "O" {
		t.Errorf("Channel.Type = %q", cfg.Channel.Type)
	}
	if cfg.Server.Path != "/hooks/zentao" {
		t.Errorf("Server.Path = %q", cfg.Server.Path)
	}
}

func TestDedupeTTL(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if got := cfg.DedupeTTL().String(); got != "10m0s" {
		t.Errorf("DedupeTTL() = %s", got)
	}
}
