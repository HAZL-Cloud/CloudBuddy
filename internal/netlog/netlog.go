// Package netlog is a small local HTTP proxy that records which hosts a
// command talked to, so a script's card can say where it went (D-065). It
// never decrypts anything: HTTPS goes through as a CONNECT tunnel, so only
// the host name is seen.
package netlog

import (
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Hit is one host and how many connections or requests went to it.
type Hit struct {
	Host  string
	Count int
}

type Log struct {
	ln   net.Listener
	srv  *http.Server
	mu   sync.Mutex
	hits map[string]int
	tr   *http.Transport
}

// Start listens on a free local port.
func Start() (*Log, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	l := &Log{ln: ln, hits: map[string]int{},
		tr: &http.Transport{Proxy: nil, DialContext: dialer.DialContext,
			ForceAttemptHTTP2: false, MaxIdleConns: 10, IdleConnTimeout: 30 * time.Second}}
	l.srv = &http.Server{Handler: l, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = l.srv.Serve(ln) }()
	return l, nil
}

// URL is the proxy address for HTTP_PROXY and HTTPS_PROXY.
func (l *Log) URL() string { return "http://" + l.ln.Addr().String() }

// Stop closes the proxy and returns the hosts, most used first. Open tunnels
// (a log tail still streaming) are cut.
func (l *Log) Stop() []Hit {
	_ = l.srv.Close()
	l.tr.CloseIdleConnections()
	return l.Hits()
}

// Hits returns the hosts seen so far, most used first.
func (l *Log) Hits() []Hit {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Hit, 0, len(l.hits))
	for h, n := range l.hits {
		out = append(out, Hit{h, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Host < out[j].Host
	})
	return out
}

func (l *Log) note(hostport, defaultPort string) {
	h := hostport
	if host, port, err := net.SplitHostPort(hostport); err == nil && port == defaultPort {
		h = host
	}
	l.mu.Lock()
	l.hits[strings.ToLower(h)]++
	l.mu.Unlock()
}

func (l *Log) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		l.tunnel(w, r)
		return
	}
	if r.URL.Host == "" {
		http.Error(w, "CloudBuddy's network log only forwards proxy requests", http.StatusBadRequest)
		return
	}
	l.note(r.URL.Host, "80")
	out := r.Clone(r.Context())
	out.RequestURI = ""
	for _, h := range []string{"Proxy-Connection", "Proxy-Authorization", "Connection", "Keep-Alive", "Te", "Trailer", "Upgrade"} {
		out.Header.Del(h)
	}
	resp, err := l.tr.RoundTrip(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(flushWriter{w}, resp.Body)
}

func (l *Log) tunnel(w http.ResponseWriter, r *http.Request) {
	target := r.Host
	if !strings.Contains(target, ":") {
		target += ":443"
	}
	l.note(target, "443")
	up, err := dialer.DialContext(r.Context(), "tcp", target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		up.Close()
		http.Error(w, "can't tunnel", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		up.Close()
		return
	}
	_, _ = conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	if n := buf.Reader.Buffered(); n > 0 {
		pending, _ := buf.Reader.Peek(n)
		_, _ = up.Write(pending)
	}
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(up, conn); closeWrite(up); done <- struct{}{} }()
	go func() { _, _ = io.Copy(conn, up); closeWrite(conn); done <- struct{}{} }()
	<-done
	<-done
	conn.Close()
	up.Close()
}

func closeWrite(c net.Conn) {
	if tc, ok := c.(interface{ CloseWrite() error }); ok {
		_ = tc.CloseWrite()
	}
}

type flushWriter struct{ w http.ResponseWriter }

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
	return n, err
}

// errLocal: the proxy never connects to this computer. It runs inside
// CloudBuddy, outside the command's sandbox, so forwarding to a local
// address would let a command reach CloudBuddy's own port (and approve its
// own requests) around the sandbox's rule (D-027). Clients skip the proxy
// for local addresses anyway (NO_PROXY).
var errLocal = errors.New("CloudBuddy's network log doesn't connect to this computer")

// dialer checks the address it actually connects to, after DNS, so a name
// that resolves to 127.0.0.1 is refused too.
var dialer = &net.Dialer{Timeout: 30 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ((ip.IsLoopback() || ip.IsUnspecified()) && !allowLocalForTests) {
		return errLocal
	}
	return nil
}}

// allowLocalForTests lets the tests forward to local test servers.
var allowLocalForTests = false

// Summary is the card text: "api.supabase.com ×3, hazl.ca".
func Summary(hits []Hit, max int) string {
	var parts []string
	for i, h := range hits {
		if i == max {
			parts = append(parts, "…")
			break
		}
		if h.Count > 1 {
			parts = append(parts, h.Host+" ×"+itoa(h.Count))
		} else {
			parts = append(parts, h.Host)
		}
	}
	return strings.Join(parts, ", ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
