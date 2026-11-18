package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"codebuddy-usage-desktop/internal/auth"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

// Client 封装鉴权头注入、401 重试与 403 透传。
type Client struct {
	manager *auth.Manager
	manual  func() string
	base    func() string
	http    *http.Client
}

func NewClient(manager *auth.Manager, manual func() string, base func() string) *Client {
	return &Client{
		manager: manager,
		manual:  manual,
		base:    base,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) APIBase() string {
	b := strings.TrimRight(c.base(), "/")
	if b == "" {
		return "https://www.workbuddy.cn"
	}
	return b
}

type response struct {
	status int
	body   []byte
}

func (c *Client) do(method, path string, headers map[string]string, body []byte) (*response, error) {
	url := c.APIBase() + path
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		authInfo, err := c.manager.Current(c.manual())
		if err != nil || authInfo == nil || authInfo.Mode == auth.ModeNone || authInfo.Token == "" {
			return nil, auth.ErrNoCredentials
		}
		req, err := http.NewRequest(method, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("authorization", "Bearer "+authInfo.Token)
		req.Header.Set("user-agent", userAgent)
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			break
		}
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			break
		}
		if resp.StatusCode == http.StatusUnauthorized {
			c.manager.Invalidate(true)
			lastErr = auth.ErrAuthExpired
			continue
		}
		return &response{status: resp.StatusCode, body: respBody}, nil
	}
	if lastErr == nil {
		lastErr = auth.ErrAuthExpired
	}
	return nil, lastErr
}

func (c *Client) DoJSON(method, path string, headers map[string]string, body interface{}, out interface{}) (int, error) {
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			return 0, err
		}
	} else {
		raw = nil
	}
	resp, err := c.do(method, path, headers, raw)
	if err != nil {
		return 0, err
	}
	if out != nil && len(resp.body) > 0 {
		if err := json.Unmarshal(resp.body, out); err != nil {
			return resp.status, fmt.Errorf("invalid JSON: %w", err)
		}
	}
	return resp.status, nil
}

func (c *Client) DoEnvelope(method, path string, headers map[string]string, body interface{}) (*APIEnvelope, error) {
	var env APIEnvelope
	if _, err := c.DoJSON(method, path, headers, body, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

func commonHeaders(apiBase, refererPath string) map[string]string {
	return map[string]string{
		"accept":            "application/json, text/plain, */*",
		"content-type":      "application/json",
		"origin":            apiBase,
		"referer":           apiBase + refererPath,
		"x-client-platform": "web",
	}
}

func (c *Client) UsageHeaders() map[string]string {
	return commonHeaders(c.APIBase(), "/profile/plans-usage")
}

func (c *Client) BuddyHeaders() map[string]string {
	return commonHeaders(c.APIBase(), "/profile/growth-center")
}

func (c *Client) CheckinHeaders() map[string]string {
	h := commonHeaders(c.APIBase(), "/profile/plans-usage")
	h["accept"] = "application/json"
	return h
}
