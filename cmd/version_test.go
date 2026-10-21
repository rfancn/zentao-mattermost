package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rfancn/zentao-mattermost/g"
)

func TestExecuteVersion(t *testing.T) {
	var out bytes.Buffer

	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"version"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	Execute()

	if !strings.Contains(out.String(), g.App) {
		t.Errorf("version 输出 = %q", out.String())
	}
}

func TestVersionCommandDefaults(t *testing.T) {
	if version == "" || commit == "" || buildDate == "" {
		t.Fatalf("版本变量不应为空: version=%q commit=%q buildDate=%q", version, commit, buildDate)
	}
}
