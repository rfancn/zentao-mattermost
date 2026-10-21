package httpclient

import (
	"testing"
	"time"

	"github.com/rfancn/zentao-mattermost/g"
)

func TestNewClientTimeout(t *testing.T) {
	g.Config.HTTP = g.HTTPConfig{TimeoutSeconds: 3}

	c := newClient()
	if c == nil {
		t.Fatal("newClient() = nil")
	}
	if got := c.GetClient().Timeout; got != 3*time.Second {
		t.Errorf("Timeout = %v, want 3s", got)
	}
}

func TestNewClientDefaultTimeout(t *testing.T) {
	g.Config.HTTP = g.HTTPConfig{}

	if got := newClient().GetClient().Timeout; got != 10*time.Second {
		t.Errorf("默认 Timeout = %v, want 10s", got)
	}
}

func TestNewReturnsSameClient(t *testing.T) {
	g.Config.HTTP = g.HTTPConfig{TimeoutSeconds: 5}

	first := New()
	second := New()
	if first != second {
		t.Error("New() 未复用已初始化的客户端")
	}
}
