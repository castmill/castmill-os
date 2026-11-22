// Package backend is the HTTP(S)/WebSocket client for the Castmill device API.
package backend

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/castmill/castmill-os/agent/internal/api"
)

// ErrUnauthorized means the backend rejected the device certificate
// (revoked or unknown device).
var ErrUnauthorized = errors.New("device not authorized by backend")

// HTTPError is a non-2xx response.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("backend: HTTP %d: %s", e.Status, e.Body) }

// Client talks to one backend.
type Client struct {
	base *url.URL
	http *http.Client
	tls  *tls.Config

	mu         sync.Mutex
	serverTime time.Time // from the last response's Date header
	timeAt     time.Time // local monotonic time when serverTime was read
}

// New creates a client. tlsCfg carries the pinned roots and the device
// certificate callback (see identity.TLSConfig).
func New(baseURL string, tlsCfg *tls.Config) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("backend URL must be http(s): %q", baseURL)
	}
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment, // proxy from castmill.conf (proxy.env)
		TLSClientConfig:     tlsCfg,
		TLSHandshakeTimeout: 15 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	return &Client{base: u, tls: tlsCfg, http: &http.Client{Transport: tr, Timeout: 60 * time.Second}}, nil
}

// ResetConnections drops pooled connections. Call it after the device
// certificate changes: a kept-alive TLS connection opened before enrollment
// carries no client certificate and would be reused.
func (c *Client) ResetConnections() { c.http.CloseIdleConnections() }

// URL returns the absolute URL for an API path.
func (c *Client) URL(path string) string { return c.base.String() + api.Prefix + path }

// ServerTime estimates the backend's current time from the last Date header.
func (c *Client) ServerTime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.serverTime.IsZero() {
		return time.Time{}
	}
	return c.serverTime.Add(time.Since(c.timeAt))
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL(path), body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	return c.send(req, out)
}

func (c *Client) send(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if d, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
		c.mu.Lock()
		c.serverTime, c.timeAt = d, time.Now()
		c.mu.Unlock()
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &HTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// Enroll starts an enrollment.
func (c *Client) Enroll(ctx context.Context, req api.EnrollmentRequest) (*api.Enrollment, error) {
	var e api.Enrollment
	return &e, c.do(ctx, http.MethodPost, "/enrollments", req, &e)
}

// Enrollment polls an enrollment.
func (c *Client) Enrollment(ctx context.Context, id string) (*api.Enrollment, error) {
	var e api.Enrollment
	return &e, c.do(ctx, http.MethodGet, "/enrollments/"+url.PathEscape(id), nil, &e)
}

// Desired returns the version the backend wants this device to run.
func (c *Client) Desired(ctx context.Context, deviceID string) (*api.Desired, error) {
	var d api.Desired
	return &d, c.do(ctx, http.MethodGet, "/devices/"+url.PathEscape(deviceID)+"/desired", nil, &d)
}

// Config returns the device configuration.
func (c *Client) Config(ctx context.Context, deviceID string) (*api.DeviceConfig, error) {
	var d api.DeviceConfig
	return &d, c.do(ctx, http.MethodGet, "/devices/"+url.PathEscape(deviceID)+"/config", nil, &d)
}

// PostStatus reports the device status.
func (c *Client) PostStatus(ctx context.Context, deviceID string, s *api.DeviceStatus) error {
	return c.do(ctx, http.MethodPost, "/devices/"+url.PathEscape(deviceID)+"/status", s, nil)
}

// RenewCertificate exchanges a CSR for a new certificate (mTLS with the old one).
func (c *Client) RenewCertificate(ctx context.Context, deviceID, csr string) (string, error) {
	var r api.CertificateResponse
	err := c.do(ctx, http.MethodPost, "/devices/"+url.PathEscape(deviceID)+"/certificate", api.CertificateRequest{CSR: csr}, &r)
	return r.Certificate, err
}

// Upload sends a blob (logs, screenshot) and returns its ID.
func (c *Client) Upload(ctx context.Context, deviceID, kind, commandID, contentType string, body io.Reader) (string, error) {
	q := url.Values{"kind": {kind}}
	if commandID != "" {
		q.Set("command_id", commandID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.URL("/devices/"+url.PathEscape(deviceID)+"/uploads")+"?"+q.Encode(), body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	var r api.UploadResponse
	return r.ID, c.send(req, &r)
}

// Conn is an open WebSocket session.
type Conn struct {
	ws *websocket.Conn
	mu sync.Mutex
}

// Dial opens the device WebSocket (mTLS).
func (c *Client) Dial(ctx context.Context, deviceID string) (*Conn, error) {
	u := *c.base
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path += api.Prefix + "/devices/" + url.PathEscape(deviceID) + "/ws"
	ws, resp, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPClient: &http.Client{Transport: c.http.Transport}})
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	ws.SetReadLimit(8 << 20)
	return &Conn{ws: ws}, nil
}

// Send writes one message (safe for concurrent use).
func (w *Conn) Send(ctx context.Context, m *api.Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return wsjson.Write(ctx, w.ws, m)
}

// Receive reads one message.
func (w *Conn) Receive(ctx context.Context) (*api.Message, error) {
	var m api.Message
	return &m, wsjson.Read(ctx, w.ws, &m)
}

// Ping checks the connection is alive.
func (w *Conn) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return w.ws.Ping(ctx)
}

// Close closes the session.
func (w *Conn) Close() error { return w.ws.Close(websocket.StatusNormalClosure, "") }
