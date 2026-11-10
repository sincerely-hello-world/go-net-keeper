package portal

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, gateway string, timeout time.Duration) *Client {
	t.Helper()
	client, err := NewClient(gateway, timeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestCheckOnline(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		status  int
		want    OnlineInfo
		wantErr bool
	}{
		{name: "online", body: `{"result":"success","userIp":"10.1.2.3"}`, want: OnlineInfo{Online: true, IP: "10.1.2.3"}},
		{name: "null IP", body: `{"userIp":null}`},
		{name: "empty IP", body: `{"userIp":""}`},
		{name: "explicit offline", body: `{"result":"fail","message":"用户未登录"}`},
		{name: "unknown failure", body: `{"result":"fail","message":"系统错误"}`, wantErr: true},
		{name: "missing IP", body: `{}`, wantErr: true},
		{name: "null response", body: `null`, wantErr: true},
		{name: "invalid IP", body: `{"userIp":"invalid"}`, wantErr: true},
		{name: "wrong IP type", body: `{"userIp":123}`, wantErr: true},
		{name: "HTML response", body: `<html>login</html>`, wantErr: true},
		{name: "HTTP error", body: `{"userIp":"10.1.2.3"}`, status: http.StatusBadGateway, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.RequestURI() != onlinePath {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.RequestURI())
				}
				if test.status != 0 {
					w.WriteHeader(test.status)
				}
				io.WriteString(w, test.body)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, time.Second)
			got, err := client.CheckOnline(context.Background())
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("online info = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestHTTPLoginFlow(t *testing.T) {
	const query = "wlanuserip=10.1.2.3&mac=aabbccddeeff&token=a%2Bb+z&note=hello world~!*()"
	const encodedQuery = "wlanuserip%3D10.1.2.3%26mac%3Daabbccddeeff%26token%3Da%252Bb%2Bz%26note%3Dhello%20world~%21%2A%28%29"
	const ciphertext = "71292489a26b05325b8498d89cc4381a02712b3c6d7c5a57f032e9a65578e66b1131b508f163cfb0dcc920439cb229302ddda8034aeca055a89f9fdb026f5f3ba64d4885f4c332621949cdfe8c2cfae2a736b55585823e2e1082f236c9f0def4bd3987b1e90a54cc710832ad61b6951311e223638a3cc5fd8f11c78b50c07318"
	var online atomic.Bool
	var loginCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "SESSION", Value: "test-session", Path: "/"})
			http.Redirect(w, r, "/landing", http.StatusFound)
		case "/landing":
			io.WriteString(w, `<script>window.location.href = '/eportal/index.jsp?`+query+`';</script>`)
		case "/eportal/InterFace.do":
			switch r.URL.Query().Get("method") {
			case "getOnlineUserInfo":
				if online.Load() {
					io.WriteString(w, `{"result":"success","userIp":"10.1.2.3"}`)
				} else {
					io.WriteString(w, `{"userIp":null}`)
				}
			case "login":
				loginCalls.Add(1)
				if r.Method != http.MethodPost {
					t.Errorf("login method = %s", r.Method)
				}
				cookie, err := r.Cookie("SESSION")
				if err != nil || cookie.Value != "test-session" {
					t.Error("login did not preserve the gateway cookie")
				}
				if got := r.Header.Get("Referer"); got != "http://"+r.Host+loginIndex+"?"+query {
					t.Errorf("unexpected Referer: %q", got)
				}
				if r.UserAgent() != userAgent {
					t.Errorf("unexpected User-Agent: %q", r.UserAgent())
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				want := map[string]string{
					"userId": "test-user", "password": ciphertext, "service": "shu",
					"passwordEncrypt": "true", "operatorPwd": "", "operatorUserId": "",
					"validcode": "", "queryString": encodedQuery,
				}
				if len(r.PostForm) != len(want) {
					t.Errorf("form field count = %d, want %d", len(r.PostForm), len(want))
				}
				for key, value := range want {
					values, ok := r.PostForm[key]
					if !ok || len(values) != 1 || values[0] != value {
						t.Errorf("form field %s differs from expected protocol value", key)
					}
				}
				online.Store(true)
				io.WriteString(w, `{"result":"success"}`)
			default:
				http.Error(w, "unknown method", http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, time.Second)
	ctx := context.Background()
	info, err := client.CheckOnline(ctx)
	if err != nil || info.Online {
		t.Fatalf("before login: info=%+v error=%v", info, err)
	}
	if err := client.Login(ctx, "test-user", "testPassword123"); err != nil {
		t.Fatal(err)
	}
	info, err = client.CheckOnline(ctx)
	if err != nil || !info.Online || info.IP != "10.1.2.3" {
		t.Fatalf("after login: info=%+v error=%v", info, err)
	}
	if loginCalls.Load() != 1 {
		t.Errorf("login calls = %d, want 1", loginCalls.Load())
	}
}

func TestLoginHTTPRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, loginIndex+"?mac=aabbccddeeff&token=x%2By", http.StatusFound)
		case loginIndex:
			io.WriteString(w, `<html>login</html>`)
		default:
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if got := r.PostForm.Get("queryString"); got != "mac%3Daabbccddeeff%26token%3Dx%252By" {
				t.Errorf("HTTP redirect lost raw query: %q", got)
			}
			io.WriteString(w, `{"result":"success"}`)
		}
	}))
	defer server.Close()
	if err := newTestClient(t, server.URL, time.Second).Login(context.Background(), "test-user", "test-password"); err != nil {
		t.Fatal(err)
	}
}

func TestLoginFailures(t *testing.T) {
	for _, test := range []struct {
		name     string
		page     string
		response string
		wantErr  error
	}{
		{name: "invalid credentials", response: `{"result":"fail","message":"用户名或密码错误"}`, wantErr: ErrInvalidCredentials},
		{name: "temporary rejection", response: `{"result":"fail","message":"服务暂不可用"}`, wantErr: ErrAuthentication},
		{name: "missing result", response: `{}`},
		{name: "invalid JSON", response: `<html>error</html>`},
		{name: "missing redirect", page: `<html>success</html>`},
		{name: "missing MAC", page: `<script>location.href="/eportal/index.jsp?token=test";</script>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" {
					page := test.page
					if page == "" {
						page = `<script>location.href="/eportal/index.jsp?mac=aabbccddeeff";</script>`
					}
					io.WriteString(w, page)
					return
				}
				posts.Add(1)
				io.WriteString(w, test.response)
			}))
			defer server.Close()
			err := newTestClient(t, server.URL, time.Second).Login(context.Background(), "test-user", "test-password")
			if err == nil {
				t.Fatal("expected login to fail")
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Errorf("error = %v, want %v", err, test.wantErr)
			}
			if test.page != "" && posts.Load() != 0 {
				t.Error("submitted credentials without valid login parameters")
			}
		})
	}
}

func TestRequestTimeoutAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, 30*time.Millisecond)
	if _, err := client.CheckOnline(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("timeout error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.CheckOnline(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("cancellation error = %v", err)
	}
}

func TestOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.Repeat(" ", maxBodySize+1))
	}))
	defer server.Close()
	if _, err := newTestClient(t, server.URL, time.Second).CheckOnline(context.Background()); err == nil {
		t.Fatal("expected oversized response to fail")
	}
}
