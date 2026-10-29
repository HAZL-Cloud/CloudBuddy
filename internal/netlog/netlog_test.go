package netlog

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRecordsPlainAndTunnelledHosts(t *testing.T) {
	allowLocalForTests = true
	defer func() { allowLocalForTests = false }()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "plain") }))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "secure") }))
	defer secure.Close()

	l, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	pu, _ := url.Parse(l.URL())
	c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	for i, u := range []string{plain.URL, secure.URL, secure.URL} {
		resp, err := c.Get(u)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if want := map[bool]string{true: "plain", false: "secure"}[i == 0]; string(b) != want {
			t.Fatalf("request %d body %q", i, b)
		}
	}
	hits := l.Stop()
	if len(hits) != 2 {
		t.Fatalf("hits %v", hits)
	}
	// HTTPS counts tunnels, not requests: a client reuses its tunnel
	ph, _ := url.Parse(plain.URL)
	sh, _ := url.Parse(secure.URL)
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.Host] = true
	}
	if !seen[ph.Host] || !seen[sh.Host] {
		t.Errorf("hits %v", hits)
	}
	if s := Summary([]Hit{{"api.supabase.com", 3}, {"hazl.ca", 1}}, 4); s != "api.supabase.com ×3, hazl.ca" {
		t.Errorf("summary %q", s)
	}
}

// The proxy runs outside the command's sandbox, so it must never connect to
// this computer: that would reach CloudBuddy's own port around the sandbox.
func TestRefusesThisComputer(t *testing.T) {
	cb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "approved") }))
	defer cb.Close()
	cu, _ := url.Parse(cb.URL)
	_, port, _ := net.SplitHostPort(cu.Host)
	l, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	pu, _ := url.Parse(l.URL())
	c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	for _, target := range []string{cb.URL, "http://localhost:" + port, "http://0.0.0.0:" + port, "https://127.0.0.1:" + port, "https://localhost:" + port} {
		resp, err := c.Get(target)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if strings.Contains(string(b), "approved") {
				t.Errorf("%s reached through the proxy", target)
			}
		}
	}
}
