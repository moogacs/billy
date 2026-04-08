package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Event is one anonymous telemetry line.
type Event struct {
	Timestamp  string                 `json:"ts"`
	Event      string                 `json:"event"`
	Success    bool                   `json:"success"`
	DurationMS int64                  `json:"duration_ms,omitempty"`
	Version    string                 `json:"version"`
	OS         string                 `json:"os"`
	Arch       string                 `json:"arch"`
	Anonymous  string                 `json:"anonymous_id"`
	Props      map[string]interface{} `json:"props,omitempty"`
}

// Client writes events locally and optionally forwards to an HTTP endpoint.
type Client struct {
	endpoint  string
	version   string
	anonID    string
	logPath   string
	http      *http.Client
	userAgent string
}

// New creates an initialized telemetry client.
func New(version, endpoint string) (*Client, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(cfg, "billy")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	id, err := ensureAnonymousID(filepath.Join(base, "anonymous_id"))
	if err != nil {
		return nil, err
	}
	return &Client{
		endpoint:  endpoint,
		version:   version,
		anonID:    id,
		logPath:   filepath.Join(base, "telemetry.jsonl"),
		http:      &http.Client{Timeout: 2 * time.Second},
		userAgent: fmt.Sprintf("billy/%s", version),
	}, nil
}

// Track writes an event locally and best-effort posts it to endpoint when set.
func (c *Client) Track(event string, success bool, d time.Duration, props map[string]interface{}) error {
	ev := Event{
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Event:      event,
		Success:    success,
		DurationMS: d.Milliseconds(),
		Version:    c.version,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Anonymous:  c.anonID,
		Props:      props,
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(c.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	_ = f.Close()

	if c.endpoint == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(b))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	_ = resp.Body.Close()
	return nil
}

func ensureAnonymousID(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		id := string(bytes.TrimSpace(b))
		if id != "" {
			return id, nil
		}
	}
	r := make([]byte, 16)
	if _, err := rand.Read(r); err != nil {
		return "", err
	}
	id := hex.EncodeToString(r)
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return id, nil
}
