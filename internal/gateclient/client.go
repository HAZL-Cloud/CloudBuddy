// Package gateclient is how the small helper programs (hook, exec wrapper,
// MCP gateway) talk to the CloudBuddy server. They are started by the agent
// CLIs with CB_GATE_URL, CB_GATE_TOKEN and CB_RUN_ID in their environment.
package gateclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Client struct {
	URL, Token, RunID string
	HTTP              *http.Client
}

func FromEnv() (*Client, error) {
	c := &Client{URL: os.Getenv("CB_GATE_URL"), Token: os.Getenv("CB_GATE_TOKEN"), RunID: os.Getenv("CB_RUN_ID"),
		HTTP: &http.Client{Timeout: 2 * time.Hour}}
	if c.URL == "" || c.Token == "" {
		return nil, fmt.Errorf("not started by CloudBuddy (CB_GATE_URL / CB_GATE_TOKEN missing)")
	}
	return c, nil
}

// Post sends body and decodes the answer into out. Any failure is an error;
// callers treat errors as "deny" (fail closed, D-018).
func (c *Client) Post(path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.URL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CB-Gate", c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("CloudBuddy isn't reachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var e struct{ Error string }
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("CloudBuddy refused: %s %s", resp.Status, e.Error)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
