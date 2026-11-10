package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NET_INTERVAL", "")
	t.Setenv("NET_TIMEOUT", "")
	for _, test := range []struct {
		name     string
		args     []string
		username string
		password string
		wantCode int
	}{
		{name: "help", args: []string{"--help"}, wantCode: 0},
		{name: "missing credentials", args: []string{"--once"}, wantCode: 2},
		{name: "missing password", username: "synthetic-user", wantCode: 2},
		{name: "whitespace username", username: "  ", password: "synthetic-password", wantCode: 2},
		{name: "zero interval", args: []string{"--interval=0s"}, wantCode: 2},
		{name: "negative timeout", args: []string{"--timeout=-1s"}, wantCode: 2},
		{name: "invalid duration", args: []string{"--interval=abc"}, wantCode: 2},
		{name: "unexpected argument", args: []string{"login"}, wantCode: 2},
		{name: "explicit missing config", args: []string{"--config=missing.toml"}, username: "synthetic-user", password: "synthetic-password", wantCode: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("NET_USERNAME", test.username)
			t.Setenv("NET_USERPASSWD", test.password)
			var output bytes.Buffer
			if got := run(test.args, &output); got != test.wantCode {
				t.Errorf("exit code = %d, want %d; output: %s", got, test.wantCode, output.String())
			}
			if strings.Contains(output.String(), "synthetic-") {
				t.Error("credentials appeared in CLI output")
			}
		})
	}
}
