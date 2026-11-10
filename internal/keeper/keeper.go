package keeper

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sincerely-hello-world/go-net-keeper/internal/portal"
)

type Portal interface {
	CheckOnline(context.Context) (portal.OnlineInfo, error)
	Login(ctx context.Context, username, password string) error
}

type Config struct {
	Username string
	Password string
	Interval time.Duration
}

type Keeper struct {
	portal Portal
	config Config
	logger *slog.Logger
}

func New(client Portal, config Config, logger *slog.Logger) (*Keeper, error) {
	if config.Username == "" || config.Password == "" {
		return nil, errors.New("请在配置文件或环境变量中设置校园网账号和密码")
	}
	if config.Interval <= 0 {
		return nil, errors.New("检查间隔必须大于 0")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Keeper{portal: client, config: config, logger: logger.With("username", config.Username)}, nil
}

// Once checks the current status and only authenticates when explicitly offline.
// A successful login response must also be confirmed by a second status check.
func (k *Keeper) Once(ctx context.Context) error {
	info, err := k.portal.CheckOnline(ctx)
	if err != nil {
		k.logger.Warn("校园网状态查询失败", "status", "unknown", "error", err)
		return err
	}
	if info.Online {
		k.logOnline(info)
		return nil
	}
	k.logger.Info("校园网未登录，开始认证", "status", "offline")
	if err := k.portal.Login(ctx, k.config.Username, k.config.Password); err != nil {
		return err
	}
	info, err = k.portal.CheckOnline(ctx)
	if err != nil {
		k.logger.Warn("登录后状态查询失败", "status", "unknown", "error", err)
		return fmt.Errorf("登录后确认在线状态: %w", err)
	}
	if !info.Online {
		return errors.New("登录接口返回成功，但尚未确认校园网在线")
	}
	k.logOnline(info)
	return nil
}

// Run performs one check immediately, then waits after each completed round.
// Checks and logins never overlap; cancellation interrupts both HTTP and waiting.
func (k *Keeper) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := k.Once(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, portal.ErrInvalidCredentials) {
			return err
		}
		if err != nil {
			k.logger.Warn("本轮维持失败，等待下次重试", "error", err, "retry_after", k.config.Interval)
		}
		timer := time.NewTimer(k.config.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (k *Keeper) logOnline(info portal.OnlineInfo) {
	k.logger.Info("校园网已登录", "status", "online", "ip", info.IP)
}
