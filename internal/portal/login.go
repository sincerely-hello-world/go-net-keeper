package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrInvalidCredentials = errors.New("校园网账号或密码错误，请检查配置文件或环境变量中的账号密码")
	ErrAuthentication     = errors.New("校园网拒绝登录")
	loginRedirect         = regexp.MustCompile(`(?i)location\s*(?:\.\s*href)?\s*=\s*['"]([^'"]+)['"]`)
)

func (c *Client) Login(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return errors.New("校园网账号和密码不能为空")
	}
	query, err := c.loginQuery(ctx)
	if err != nil {
		return err
	}
	mac, err := extractMAC(query)
	if err != nil {
		return err
	}
	encrypted, err := EncryptPassword(password + ">" + mac)
	if err != nil {
		return fmt.Errorf("加密校园网密码: %w", err)
	}
	form := url.Values{
		"userId":          {username},
		"password":        {encrypted},
		"service":         {"shu"},
		"passwordEncrypt": {"true"},
		"operatorPwd":     {""},
		"operatorUserId":  {""},
		"validcode":       {""},
		// Rust urlencoding::encode uses percent escapes, including %20 for
		// spaces. Values.Encode then applies the second (form) encoding.
		"queryString": {strings.ReplaceAll(url.QueryEscape(query), "+", "%20")},
	}
	body, _, err := c.request(ctx, http.MethodPost, loginPath,
		strings.NewReader(form.Encode()), c.gateway+loginIndex+"?"+query)
	if err != nil {
		return fmt.Errorf("提交校园网登录: %w", err)
	}
	var response struct {
		Result  string `json:"result"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("解析校园网登录响应: %w", err)
	}
	if response.Result == "success" {
		return nil
	}
	if response.Result == "" {
		return errors.New("校园网登录响应缺少 result")
	}
	message := strings.ToLower(response.Message)
	for _, text := range []string{
		"密码错误", "密码不正确", "用户名或密码错误", "用户名不存在", "用户不存在",
		"账号不存在", "帐号不存在", "invalid username or password", "incorrect password", "invalid password",
	} {
		if strings.Contains(message, text) {
			return ErrInvalidCredentials
		}
	}
	// Do not log the raw response: a gateway may echo credentials or session data.
	return ErrAuthentication
}

func (c *Client) loginQuery(ctx context.Context) (string, error) {
	body, finalURL, err := c.request(ctx, http.MethodGet, "/", nil, "")
	if err != nil {
		return "", fmt.Errorf("获取校园网登录参数: %w", err)
	}
	// HTTP redirects may have already reached the login page.
	if _, err := extractMAC(finalURL.RawQuery); err == nil {
		return finalURL.RawQuery, nil
	}
	match := loginRedirect.FindSubmatch(body)
	if len(match) != 2 {
		return "", errors.New("校园网页面中未找到登录跳转地址")
	}
	u, err := url.Parse(string(match[1]))
	if err != nil {
		return "", errors.New("校园网登录跳转地址无效")
	}
	if _, err := extractMAC(u.RawQuery); err != nil {
		return "", err
	}
	return u.RawQuery, nil
}

func extractMAC(query string) (string, error) {
	// Preserve the raw value, matching Rust: do not decode or re-encode the
	// gateway's query string before appending the MAC to the password.
	for pair := range strings.SplitSeq(query, "&") {
		key, value, ok := strings.Cut(pair, "=")
		if ok && key == "mac" && value != "" {
			return value, nil
		}
	}
	return "", errors.New("校园网登录参数缺少 mac")
}
