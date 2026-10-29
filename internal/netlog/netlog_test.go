package netlog

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRecordsPlainAndTunnelledHosts(t *testing.T) {
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
