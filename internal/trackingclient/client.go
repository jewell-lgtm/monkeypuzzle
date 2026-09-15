// Package trackingclient implements the explicitly invoked mp-server client.
package trackingclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
)

type Client struct {
	endpoint, token string
	http            *http.Client
}

func New(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("tracking is opt-in: set --server or MP_SERVER_URL to an http(s) base URL without credentials, query or fragment")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("tracking requires MP_SERVER_TOKEN (an mp-server OAuth access token)")
	}
	return &Client{endpoint: strings.TrimRight(endpoint, "/"), token: token, http: &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) call(ctx context.Context, method, path string, payload, result any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if len(data) > tracking.MaxBody {
			return fmt.Errorf("snapshot exceeds 64 KiB")
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("mp-server request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	expected := http.StatusOK
	if method == http.MethodDelete {
		expected = http.StatusNoContent
	}
	if resp.StatusCode != expected {
		// Do not echo an arbitrary remote body: it can contain credentials or HTML.
		return fmt.Errorf("mp-server returned HTTP %d", resp.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(result)
	}
	return nil
}

func (c *Client) Put(ctx context.Context, key tracking.Key, snapshot tracking.Snapshot) (tracking.Item, error) {
	var item tracking.Item
	if err := key.Validate(); err != nil {
		return item, err
	}
	if err := snapshot.Validate(); err != nil {
		return item, err
	}
	err := c.call(ctx, http.MethodPut, key.Path(), snapshot, &item)
	return item, err
}
func (c *Client) Delete(ctx context.Context, key tracking.Key) error {
	if err := key.Validate(); err != nil {
		return err
	}
	return c.call(ctx, http.MethodDelete, key.Path(), nil, nil)
}
func (c *Client) List(ctx context.Context) (tracking.List, error) {
	var items tracking.List
	err := c.call(ctx, http.MethodGet, tracking.BasePath, nil, &items)
	return items, err
}
