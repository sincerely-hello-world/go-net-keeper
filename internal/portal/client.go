package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultGateway = "http://10.10.9.9"
	onlinePath     = "/eportal/InterFace.do?method=getOnlineUserInfo"
	loginPath      = "/eportal/InterFace.do?method=login"
	loginIndex     = "/eportal/index.jsp"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/79.0.3945.88 Safari/537.36"
	maxBodySize    = 1 << 20
)

type OnlineInfo struct {
	Online bool
	IP     string
}

// Client keeps cookies and connections across checks and authentication.
// Use one Client from the keeper's serial loop.
type Client struct {
	gateway string
	http    *http.Client
}

func NewClient(gateway string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(gateway)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") ||
		u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") {
		return nil, errors.New("网关必须是有效的 HTTP/HTTPS 源地址")
	}
	if timeout <= 0 {
		return nil, errors.New("请求超时必须大于 0")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("创建 Cookie 存储: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// The campus gateway is a local endpoint; do not send it through a system proxy.
	transport.Proxy = nil
	return &Client{
		gateway: strings.TrimRight(gateway, "/"),
		http: &http.Client{
			Timeout:   timeout,
			Jar:       jar,
			Transport: transport,
		},
	}, nil
}

func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

func (c *Client) CheckOnline(ctx context.Context) (OnlineInfo, error) {
	body, _, err := c.request(ctx, http.MethodGet, onlinePath, nil, "")
	if err != nil {
		return OnlineInfo{}, fmt.Errorf("查询在线状态: %w", err)
	}
	var response struct {
		UserIP  json.RawMessage `json:"userIp"`
		Result  string          `json:"result"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return OnlineInfo{}, fmt.Errorf("解析在线状态: %w", err)
	}
	if len(response.UserIP) == 0 {
		// Some portal versions omit userIp when explicitly reporting a logout.
		if (response.Result == "fail" || response.Result == "failed") &&
			(strings.Contains(response.Message, "未登录") || strings.Contains(response.Message, "已下线")) {
			return OnlineInfo{}, nil
		}
		return OnlineInfo{}, errors.New("在线状态响应缺少 userIp，无法判断是否需要登录")
	}
	var ip *string
	if err := json.Unmarshal(response.UserIP, &ip); err != nil {
		return OnlineInfo{}, fmt.Errorf("解析在线 IP: %w", err)
	}
	if ip == nil || *ip == "" {
		return OnlineInfo{}, nil
	}
	if _, err := netip.ParseAddr(*ip); err != nil {
		return OnlineInfo{}, errors.New("在线状态响应中的 IP 地址无效")
	}
	return OnlineInfo{Online: true, IP: *ip}, nil
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, referer string) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.gateway+path, body)
	if err != nil {
		return nil, nil, errors.New("创建校园网请求失败")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	response, err := c.http.Do(req)
	if err != nil {
		// url.Error includes the complete URL, which may contain session data.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, nil, fmt.Errorf("校园网请求失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("校园网返回 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBodySize+1))
	if err != nil {
		return nil, nil, fmt.Errorf("读取校园网响应: %w", err)
	}
	if len(data) > maxBodySize {
		return nil, nil, errors.New("校园网响应超过 1 MiB")
	}
	return data, response.Request.URL, nil
}
