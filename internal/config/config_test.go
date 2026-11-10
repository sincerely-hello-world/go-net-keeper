package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	for _, test := range []struct {
		name     string
		file     *string
		env      map[string]string
		required bool
		want     Config
		wantErr  bool
	}{
		{
			name: "file overrides environment for every field",
			file: text("username = 'file-user'\npassword = 'file-password'\ninterval = '30s'\ntimeout = '5s'"),
			env:  map[string]string{"NET_USERNAME": "env-user", "NET_USERPASSWD": "env-password", "NET_INTERVAL": "1m", "NET_TIMEOUT": "20s"},
			want: Config{Username: "file-user", Password: "file-password", Interval: 30 * time.Second, Timeout: 5 * time.Second},
		},
		{
			name: "partial file falls back per field",
			file: text("username = 'file-user'\ninterval = '30s'"),
			env:  map[string]string{"NET_USERNAME": "env-user", "NET_USERPASSWD": "env-password", "NET_INTERVAL": "1m", "NET_TIMEOUT": "20s"},
			want: Config{Username: "file-user", Password: "env-password", Interval: 30 * time.Second, Timeout: 20 * time.Second},
		},
		{
			name: "optional missing file uses environment and defaults",
			env:  map[string]string{"NET_USERNAME": " env-user ", "NET_USERPASSWD": "env-password"},
			want: Config{Username: "env-user", Password: "env-password", Interval: 10 * time.Second, Timeout: 10 * time.Second},
		},
		{
			name: "missing file uses environment durations",
			env:  map[string]string{"NET_USERNAME": "env-user", "NET_USERPASSWD": "env-password", "NET_INTERVAL": "1m", "NET_TIMEOUT": "3s"},
			want: Config{Username: "env-user", Password: "env-password", Interval: time.Minute, Timeout: 3 * time.Second},
		},
		{
			name: "invalid environment values are ignored when file overrides them",
			file: text("username = 'file-user'\npassword = 'file-password'\ninterval = '30s'\ntimeout = '5s'"),
			env:  map[string]string{"NET_INTERVAL": "invalid", "NET_TIMEOUT": "invalid"},
			want: Config{Username: "file-user", Password: "file-password", Interval: 30 * time.Second, Timeout: 5 * time.Second},
		},
		{
			name: "password whitespace is preserved",
			file: text("username = ' file-user '\npassword = ' password with spaces '"),
			want: Config{Username: "file-user", Password: " password with spaces ", Interval: 10 * time.Second, Timeout: 10 * time.Second},
		},
		{name: "explicit missing file fails", required: true, env: map[string]string{"NET_USERNAME": "env-user", "NET_USERPASSWD": "env-password"}, wantErr: true},
		{name: "missing credentials fails", wantErr: true},
		{name: "empty file password does not fall back", file: text("username = 'file-user'\npassword = ''"), env: map[string]string{"NET_USERPASSWD": "env-password"}, wantErr: true},
		{name: "blank username fails", file: text("username = ' '\npassword = 'file-password'"), wantErr: true},
		{name: "zero interval fails", file: text("username = 'file-user'\npassword = 'file-password'\ninterval = '0s'"), wantErr: true},
		{name: "negative timeout fails", file: text("username = 'file-user'\npassword = 'file-password'\ntimeout = '-1s'"), wantErr: true},
		{name: "invalid environment duration fails", env: map[string]string{"NET_USERNAME": "env-user", "NET_USERPASSWD": "env-password", "NET_INTERVAL": "invalid"}, wantErr: true},
		{name: "invalid TOML fails without leaking password", file: text("username = 'file-user'\npassword = 'file-password"), wantErr: true},
		{name: "wrong field type fails", file: text("username = 'file-user'\npassword = 'file-password'\ninterval = 10"), wantErr: true},
		{name: "unknown field fails", file: text("username = 'file-user'\npassword = 'file-password'\ntimout = '5s'"), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, key := range []string{"NET_USERNAME", "NET_USERPASSWD", "NET_INTERVAL", "NET_TIMEOUT"} {
				t.Setenv(key, test.env[key])
			}
			path := filepath.Join(t.TempDir(), "config.toml")
			if test.file != nil {
				if err := os.WriteFile(path, []byte(*test.file), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := Load(path, test.required)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
			if err != nil {
				if strings.Contains(err.Error(), "file-password") || strings.Contains(err.Error(), "env-password") {
					t.Fatal("password appeared in configuration error")
				}
				return
			}
			if got != test.want {
				t.Error("merged configuration differs from expected fields")
			}
		})
	}
}

func TestUnreadableConfigDoesNotFallBack(t *testing.T) {
	if _, err := Load(t.TempDir(), false); err == nil {
		t.Fatal("expected a directory used as a config file to fail")
	}
}

func text(value string) *string { return &value }
