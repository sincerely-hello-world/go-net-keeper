package keeper

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sincerely-hello-world/go-net-keeper/internal/portal"
)

type checkResult struct {
	info portal.OnlineInfo
	err  error
}

type fakePortal struct {
	checks   []checkResult
	loginErr error
	calls    []string
	onCheck  func()
	username string
	password string
}

func (p *fakePortal) CheckOnline(context.Context) (portal.OnlineInfo, error) {
	p.calls = append(p.calls, "check")
	if p.onCheck != nil {
		p.onCheck()
	}
	if len(p.checks) == 0 {
		return portal.OnlineInfo{}, errors.New("unexpected extra status check")
	}
	result := p.checks[0]
	p.checks = p.checks[1:]
	return result.info, result.err
}

func (p *fakePortal) Login(_ context.Context, username, password string) error {
	p.calls = append(p.calls, "login")
	p.username, p.password = username, password
	return p.loginErr
}

func newTestKeeper(t *testing.T, p *fakePortal, interval time.Duration) (*Keeper, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	k, err := New(p, Config{
		Username: "synthetic-user", Password: "synthetic-password", Interval: interval,
	}, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return k, &logs
}

func TestOnce(t *testing.T) {
	networkErr := errors.New("network unavailable")
	online := checkResult{info: portal.OnlineInfo{Online: true, IP: "10.1.2.3"}}
	for _, test := range []struct {
		name     string
		checks   []checkResult
		loginErr error
		wantErr  bool
		wantIs   error
		calls    []string
	}{
		{name: "already online", checks: []checkResult{online}, calls: []string{"check"}},
		{name: "offline then login", checks: []checkResult{{}, online}, calls: []string{"check", "login", "check"}},
		{name: "unknown status does not login", checks: []checkResult{{err: networkErr}}, wantErr: true, wantIs: networkErr, calls: []string{"check"}},
		{name: "credentials rejected", checks: []checkResult{{}}, loginErr: portal.ErrInvalidCredentials, wantErr: true, wantIs: portal.ErrInvalidCredentials, calls: []string{"check", "login"}},
		{name: "login success but still offline", checks: []checkResult{{}, {}}, wantErr: true, calls: []string{"check", "login", "check"}},
		{name: "confirmation unavailable", checks: []checkResult{{}, {err: networkErr}}, wantErr: true, wantIs: networkErr, calls: []string{"check", "login", "check"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := &fakePortal{checks: test.checks, loginErr: test.loginErr}
			k, logs := newTestKeeper(t, p, time.Second)
			err := k.Once(context.Background())
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
			if test.wantIs != nil && !errors.Is(err, test.wantIs) {
				t.Errorf("error = %v, want %v", err, test.wantIs)
			}
			if !reflect.DeepEqual(p.calls, test.calls) {
				t.Errorf("calls = %v, want %v", p.calls, test.calls)
			}
			if p.username != "" && (p.username != "synthetic-user" || p.password != "synthetic-password") {
				t.Error("login did not receive configured credentials")
			}
			if strings.Contains(logs.String(), "synthetic-password") {
				t.Error("password appeared in logs")
			}
			if !strings.Contains(logs.String(), "username=synthetic-user") {
				t.Error("status log did not include configured username")
			}
			if test.name == "unknown status does not login" && !strings.Contains(logs.String(), "status=unknown") {
				t.Error("failed status check did not report unknown state")
			}
		})
	}
}

func TestRunRetriesTransientFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p := &fakePortal{checks: []checkResult{
		{err: errors.New("network unavailable")},
		{},
		{info: portal.OnlineInfo{Online: true, IP: "10.1.2.3"}},
	}}
	p.onCheck = func() {
		if len(p.calls) == 4 {
			cancel()
		}
	}
	k, _ := newTestKeeper(t, p, time.Millisecond)
	if err := k.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"check", "check", "login", "check"}; !reflect.DeepEqual(p.calls, want) {
		t.Errorf("calls = %v, want %v", p.calls, want)
	}
}

func TestRunStopsOnInvalidCredentials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p := &fakePortal{checks: []checkResult{{}}, loginErr: portal.ErrInvalidCredentials}
	k, _ := newTestKeeper(t, p, time.Millisecond)
	if err := k.Run(ctx); !errors.Is(err, portal.ErrInvalidCredentials) {
		t.Fatalf("error = %v, want invalid credentials", err)
	}
	if want := []string{"check", "login"}; !reflect.DeepEqual(p.calls, want) {
		t.Errorf("calls = %v, want %v", p.calls, want)
	}
}

func TestRunCancellationInterruptsWaiting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	p := &fakePortal{checks: []checkResult{{info: portal.OnlineInfo{Online: true, IP: "10.1.2.3"}}}}
	k, _ := newTestKeeper(t, p, time.Hour)
	if err := k.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"check"}; !reflect.DeepEqual(p.calls, want) {
		t.Errorf("calls = %v, want %v", p.calls, want)
	}
}

func TestEveryCheckLogsOnlineStatusAndUsername(t *testing.T) {
	online := checkResult{info: portal.OnlineInfo{Online: true, IP: "10.1.2.3"}}
	p := &fakePortal{checks: []checkResult{online, online}}
	k, logs := newTestKeeper(t, p, time.Second)
	for range 2 {
		if err := k.Once(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Count(logs.String(), "校园网已登录"); got != 2 {
		t.Errorf("online log count = %d, want 2", got)
	}
	for _, field := range []string{"username=synthetic-user", "status=online", "ip=10.1.2.3"} {
		if got := strings.Count(logs.String(), field); got != 2 {
			t.Errorf("status field %q count = %d, want 2", field, got)
		}
	}
}
