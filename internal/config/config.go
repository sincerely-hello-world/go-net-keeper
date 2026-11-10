package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const DefaultPath = "config.toml"

type Config struct {
	Username string
	Password string
	Interval time.Duration
	Timeout  time.Duration
}

type fileConfig struct {
	Username *string `toml:"username"`
	Password *string `toml:"password"`
	Interval *string `toml:"interval"`
	Timeout  *string `toml:"timeout"`
}

// Load merges each field in order: file > environment > default.
// A missing default file is optional; an explicitly selected file is required.
// Explicitly empty or invalid file fields fail validation instead of falling back.
func Load(path string, required bool) (Config, error) {
	var file fileConfig
	data, err := os.ReadFile(path)
	if err != nil && !(errors.Is(err, os.ErrNotExist) && !required) {
		return Config{}, fmt.Errorf("读取配置文件 %q: %w", path, err)
	}
	if err == nil {
		decoder := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
		if err := decoder.Decode(&file); err != nil {
			// TOML diagnostics may contain the password's source line. Only
			// report its position, never the raw diagnostic or source text.
			var decodeErr *toml.DecodeError
			if errors.As(err, &decodeErr) {
				row, column := decodeErr.Position()
				return Config{}, fmt.Errorf("配置文件 %q 格式无效（第 %d 行，第 %d 列），请检查字段和类型", path, row, column)
			}
			return Config{}, fmt.Errorf("配置文件 %q 格式无效，请检查 TOML 语法和字段", path)
		}
	}

	cfg := Config{
		Username: strings.TrimSpace(resolve(file.Username, "NET_USERNAME", "")),
		Password: resolve(file.Password, "NET_USERPASSWD", ""),
	}
	if cfg.Username == "" || cfg.Password == "" {
		return Config{}, errors.New("请在 config.toml 配置 username、password，或设置 NET_USERNAME、NET_USERPASSWD 环境变量")
	}
	cfg.Interval, err = parseDuration("interval", resolve(file.Interval, "NET_INTERVAL", "10s"))
	if err != nil {
		return Config{}, err
	}
	cfg.Timeout, err = parseDuration("timeout", resolve(file.Timeout, "NET_TIMEOUT", "10s"))
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func resolve(value *string, env, fallback string) string {
	if value != nil {
		return *value
	}
	if value := os.Getenv(env); value != "" {
		return value
	}
	return fallback
}

func parseDuration(name, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s 必须是大于 0 的时长，例如 10s 或 1m", name)
	}
	return duration, nil
}
