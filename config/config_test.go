package config

import (
	"errors"
	"testing"

	"github.com/rfancn/zentao-mattermost/internal/errdef"
)

const testRouteName = "zentao-mattermost-bug"

func testRoute() RouteConfig {
	return RouteConfig{
		Source: SourceConfig{Type: "zentao", URL: "http://zentao/", Account: "admin", Password: "password"},
		Target: TargetConfig{Type: "mattermost", URL: "http://mattermost/", BotToken: "mm"},
	}
}

func validConfig() AppConfig {
	return AppConfig{
		Token: "secret",
		Routes: map[string]RouteConfig{
			testRouteName: testRoute(),
		},
	}
}

func TestNormalizeDefaults(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if cfg.Listen != ":8080" {
		t.Errorf("Listen = %q", cfg.Listen)
	}
	if cfg.Routes[testRouteName].Source.URL != "http://zentao" {
		t.Errorf("Source.URL = %q", cfg.Routes[testRouteName].Source.URL)
	}
	if cfg.Routes[testRouteName].Target.URL != "http://mattermost" {
		t.Errorf("Target.URL = %q", cfg.Routes[testRouteName].Target.URL)
	}
}

func TestNormalizeRouteName(t *testing.T) {
	cfg := validConfig()
	cfg.Routes = map[string]RouteConfig{" zentao-mattermost-bug ": testRoute()}

	if err := cfg.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if _, ok := cfg.Routes["zentao-mattermost-bug"]; !ok {
		t.Errorf("Routes = %v, want key zentao-mattermost-bug", cfg.Routes)
	}
}

func TestNormalizeRouteNameInvalid(t *testing.T) {
	for _, name := range []string{"foo bar", "foo?bar", "foo#bar", "foo/bar", "foo.bar", "/foo", "foo/../bar"} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Routes = map[string]RouteConfig{name: testRoute()}

			if err := cfg.Normalize(); !errors.Is(err, errdef.ErrRouteNameInvalid) {
				t.Fatalf("Normalize() error = %v, want ErrRouteNameInvalid", err)
			}
		})
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
			mutate:  func(c *AppConfig) { c.Token = "" },
			wantErr: errdef.ErrServerTokenRequired,
		},
		{
			name:    "missing routes",
			mutate:  func(c *AppConfig) { c.Routes = nil },
			wantErr: errdef.ErrRoutesRequired,
		},
		{
			name:    "missing route name",
			mutate:  func(c *AppConfig) { c.Routes[""] = testRoute() },
			wantErr: errdef.ErrRouteNameRequired,
		},
		{
			name: "duplicated route name",
			mutate: func(c *AppConfig) {
				c.Routes[" "+testRouteName] = testRoute()
			},
			wantErr: errdef.ErrRouteNameDuplicated,
		},
		{
			name: "missing source type",
			mutate: func(c *AppConfig) {
				route := c.Routes[testRouteName]
				route.Source.Type = ""
				c.Routes[testRouteName] = route
			},
			wantErr: errdef.ErrSourceTypeRequired,
		},
		{
			name: "missing target type",
			mutate: func(c *AppConfig) {
				route := c.Routes[testRouteName]
				route.Target.Type = ""
				c.Routes[testRouteName] = route
			},
			wantErr: errdef.ErrTargetTypeRequired,
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
