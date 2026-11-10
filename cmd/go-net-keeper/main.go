package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sincerely-hello-world/go-net-keeper/internal/config"
	"github.com/sincerely-hello-world/go-net-keeper/internal/keeper"
	"github.com/sincerely-hello-world/go-net-keeper/internal/portal"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, output io.Writer) int {
	flags := flag.NewFlagSet("go-net-keeper", flag.ContinueOnError)
	flags.SetOutput(output)
	configPath := flags.String("config", config.DefaultPath, "配置文件路径；默认文件不存在时使用环境变量")
	interval := flags.Duration("interval", 10*time.Second, "每轮完成后的检查间隔，例如 10s、1m")
	timeout := flags.Duration("timeout", 10*time.Second, "单次 HTTP 请求超时")
	once := flags.Bool("once", false, "只检查一次，必要时登录并确认，然后退出")
	flags.Usage = func() {
		fmt.Fprintln(output, "用法: go-net-keeper [选项]")
		fmt.Fprintln(output, "配置优先读取 config.toml，缺少的字段从 NET_USERNAME、NET_USERPASSWD、NET_INTERVAL、NET_TIMEOUT 环境变量读取。")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(output, "不接受位置参数；使用 --help 查看用法")
		return 2
	}
	if *interval <= 0 || *timeout <= 0 {
		fmt.Fprintln(output, "interval 和 timeout 必须大于 0")
		return 2
	}
	var configRequired, intervalSet, timeoutSet bool
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "config":
			configRequired = true
		case "interval":
			intervalSet = true
		case "timeout":
			timeoutSet = true
		}
	})
	cfg, err := config.Load(*configPath, configRequired)
	if err != nil {
		fmt.Fprintln(output, err)
		return 2
	}
	if intervalSet {
		cfg.Interval = *interval
	}
	if timeoutSet {
		cfg.Timeout = *timeout
	}
	client, err := portal.NewClient(portal.DefaultGateway, cfg.Timeout)
	if err != nil {
		fmt.Fprintln(output, err)
		return 2
	}
	defer client.Close()
	logger := slog.New(slog.NewTextHandler(output, nil))
	worker, err := keeper.New(client, keeper.Config{
		Username: cfg.Username,
		Password: cfg.Password,
		Interval: cfg.Interval,
	}, logger)
	logger = logger.With("username", cfg.Username)
	if err != nil {
		logger.Error("配置无效", "error", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *once {
		err = worker.Once(ctx)
	} else {
		logger.Info("启动校园网维持", "interval", cfg.Interval, "timeout", cfg.Timeout)
		err = worker.Run(ctx)
	}
	if ctx.Err() != nil {
		logger.Info("已停止校园网维持")
		return 0
	}
	if err != nil {
		logger.Error("校园网维持失败", "error", err)
		return 1
	}
	return 0
}
