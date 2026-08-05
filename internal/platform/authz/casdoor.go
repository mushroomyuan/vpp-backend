package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// PermissionSource fetches Casdoor permissions (test seam).
type PermissionSource interface {
	FetchPermissions(ctx context.Context, owner string) ([]RemotePermission, error)
}

// CasdoorClientConfig is admin-API access for B1 pull.
type CasdoorClientConfig struct {
	BaseURL      string // e.g. http://127.0.0.1:8000
	Organization string // login org, default built-in
	Application  string // login app, default app-built-in
	Username     string // default admin
	Password     string
	HTTPClient   *http.Client
}

// CasdoorClient talks to Casdoor management APIs with a session cookie.
// Only this adapter knows Casdoor HTTP shapes; do not import it from usecase.
type CasdoorClient struct {
	cfg    CasdoorClientConfig
	client *http.Client
}

// NewCasdoorClient builds a client with an isolated cookie jar.
func NewCasdoorClient(cfg CasdoorClientConfig) (*CasdoorClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("authz casdoor: BaseURL required")
	}
	if cfg.Organization == "" {
		cfg.Organization = "built-in"
	}
	if cfg.Application == "" {
		cfg.Application = "app-built-in"
	}
	if cfg.Username == "" {
		cfg.Username = "admin"
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("authz casdoor: Password required")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	// Clone-like: ensure jar is set without mutating caller's client unexpectedly.
	client := *hc
	client.Jar = jar
	return &CasdoorClient{cfg: cfg, client: &client}, nil
}

type casdoorAPIResponse struct {
	Status string          `json:"status"`
	Msg    string          `json:"msg"`
	Data   json.RawMessage `json:"data"`
}

// FetchPermissions implements PermissionSource via GET /api/get-permissions.
func (c *CasdoorClient) FetchPermissions(ctx context.Context, owner string) ([]RemotePermission, error) {
	if err := c.ensureLogin(ctx); err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimRight(c.cfg.BaseURL, "/") + "/api/get-permissions")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("owner", owner)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		// Session may have expired; one retry after re-login.
		if err := c.login(ctx); err != nil {
			return nil, err
		}
		req2, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		res2, err := c.client.Do(req2)
		if err != nil {
			return nil, err
		}
		defer res2.Body.Close()
		body, err = io.ReadAll(res2.Body)
		if err != nil {
			return nil, err
		}
		res = res2
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authz casdoor get-permissions: HTTP %d: %s", res.StatusCode, truncate(body, 200))
	}
	var wrap casdoorAPIResponse
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("authz casdoor get-permissions decode: %w", err)
	}
	if wrap.Status != "" && wrap.Status != "ok" {
		return nil, fmt.Errorf("authz casdoor get-permissions: status=%s msg=%s", wrap.Status, wrap.Msg)
	}
	if len(wrap.Data) == 0 || string(wrap.Data) == "null" {
		return nil, nil
	}
	var perms []RemotePermission
	if err := json.Unmarshal(wrap.Data, &perms); err != nil {
		return nil, fmt.Errorf("authz casdoor permissions data: %w", err)
	}
	return perms, nil
}

func (c *CasdoorClient) ensureLogin(ctx context.Context) error {
	base, err := url.Parse(c.cfg.BaseURL)
	if err != nil {
		return err
	}
	if len(c.client.Jar.Cookies(base)) > 0 {
		return nil
	}
	return c.login(ctx)
}

func (c *CasdoorClient) login(ctx context.Context) error {
	payload := map[string]any{
		"application":  c.cfg.Application,
		"organization": c.cfg.Organization,
		"username":     c.cfg.Username,
		"password":     c.cfg.Password,
		"type":         "login",
		"autoSignin":   true,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/api/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var wrap casdoorAPIResponse
	if err := json.Unmarshal(body, &wrap); err != nil {
		return fmt.Errorf("authz casdoor login decode: %w", err)
	}
	if wrap.Status != "ok" {
		return fmt.Errorf("authz casdoor login failed: status=%s msg=%s", wrap.Status, wrap.Msg)
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
