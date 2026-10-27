// Package server is CloudBuddy's HTTP side: the browser API (docs/api.md),
// live updates, the gate endpoints used by the hook, exec wrapper and MCP
// gateway, and the local-security rules of section 4.9.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"cloudbuddy/internal/bus"
	"cloudbuddy/internal/config"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/run"
	"cloudbuddy/internal/store"
)

type Agent struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Version   string `json:"version"`
	Available bool   `json:"available"`
}

type Server struct {
	cfg     config.Config
	eng     *run.Engine
	bus     *bus.Bus
	web     fs.FS // built frontend; nil shows a "not built" page
	agents  func() []Agent
	scan    func(ctx context.Context) error
	devPort int

	mu       sync.Mutex
	launch   map[string]time.Time // one-time launch codes
	sessions map[string]string    // session cookie -> csrf token
}

func New(cfg config.Config, eng *run.Engine, b *bus.Bus, web fs.FS, agents func() []Agent, scan func(context.Context) error) *Server {
	s := &Server{cfg: cfg, eng: eng, bus: b, web: web, agents: agents, scan: scan,
		launch: map[string]time.Time{}, sessions: map[string]string{}}
	if cfg.Dev {
		s.devPort = 5173
	}
	return s
}

// NewLaunchCode returns a code for the URL CloudBuddy opens in the browser.
// It works once and expires after 60 seconds.
func (s *Server) NewLaunchCode() string {
	code := config.Random(16)
	s.mu.Lock()
	s.launch[code] = time.Now().Add(60 * time.Second)
	s.mu.Unlock()
	return code
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// browser
	mux.HandleFunc("GET /launch/{code}", s.handleLaunch)
	mux.HandleFunc("GET /api/session", s.auth(s.handleSession))
	mux.HandleFunc("GET /api/conversations", s.auth(s.handleList))
	mux.HandleFunc("POST /api/conversations", s.auth(s.handleCreate))
	mux.HandleFunc("GET /api/conversations/{id}", s.auth(s.handleDetail))
	mux.HandleFunc("POST /api/conversations/{id}/messages", s.auth(s.handleMessage))
	mux.HandleFunc("POST /api/conversations/{id}/stop", s.auth(s.handleStop))
	mux.HandleFunc("POST /api/approvals/{id}", s.auth(s.handleDecide))
	mux.HandleFunc("GET /api/connections", s.auth(s.handleConnections))
	mux.HandleFunc("POST /api/connections/scan", s.auth(s.handleScan))
	mux.HandleFunc("PUT /api/connections/{id}/rule", s.auth(s.handleRule))
	mux.HandleFunc("GET /stream", s.auth(s.handleStream))
	// gate (hook, exec wrapper, MCP gateway): separate secret, can't approve
	mux.HandleFunc("POST /gate/claude/pretool", s.gate(s.handlePreTool))
	mux.HandleFunc("POST /gate/claude/session", s.gate(s.handleSessionStart))
	mux.HandleFunc("POST /gate/release", s.gate(s.handleRelease))
	mux.HandleFunc("POST /gate/exec/finished", s.gate(s.handleExecFinished))
	mux.HandleFunc("POST /gate/mcp/tools", s.gate(s.handleMCPTools))
	mux.HandleFunc("POST /gate/mcp/call", s.gate(s.handleMCPCall))
	mux.HandleFunc("POST /gate/mcp/result", s.gate(s.handleMCPResult))
	mux.HandleFunc("GET /", s.handleStatic)
	return s.guard(mux)
}

// ---------- local security (section 4.9) ----------

func (s *Server) allowedHosts() []string {
	h := []string{fmt.Sprintf("127.0.0.1:%d", s.cfg.Port), fmt.Sprintf("localhost:%d", s.cfg.Port)}
	if s.devPort != 0 {
		h = append(h, fmt.Sprintf("127.0.0.1:%d", s.devPort), fmt.Sprintf("localhost:%d", s.devPort))
	}
	return h
}

func (s *Server) allowedOrigin(o string) bool {
	for _, h := range s.allowedHosts() {
		if o == "http://"+h {
			return true
		}
	}
	return false
}

// guard rejects requests for other host names (DNS rebinding) and sets
// security headers. It never sends CORS headers.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := false
		for _, h := range s.allowedHosts() {
			if r.Host == h {
				ok = true
			}
		}
		if !ok {
			http.Error(w, "wrong host", http.StatusMisdirectedRequest)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
		h.Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !strings.HasPrefix(r.URL.Path, "/gate/") {
			if !s.allowedOrigin(r.Header.Get("Origin")) {
				writeErr(w, http.StatusForbidden, "bad_origin")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	s.mu.Lock()
	exp, ok := s.launch[code]
	delete(s.launch, code)
	s.mu.Unlock()
	if !ok || time.Now().After(exp) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>CloudBuddy</title><body style="font-family:system-ui;padding:40px">This link was already used or has expired. Run <code>cloudbuddy</code> again, or open the newest link it printed.</body>`)
		return
	}
	sid := config.Random(24)
	s.mu.Lock()
	s.sessions[sid] = config.Random(24)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "cb_session", Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 3600})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("cb_session")
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not_signed_in")
			return
		}
		s.mu.Lock()
		csrf, ok := s.sessions[c.Value]
		s.mu.Unlock()
		if !ok {
			writeErr(w, http.StatusUnauthorized, "not_signed_in")
			return
		}
		if r.Method != http.MethodGet && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CB-CSRF")), []byte(csrf)) != 1 {
			writeErr(w, http.StatusForbidden, "bad_csrf")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), csrfKey{}, csrf)))
	}
}

type csrfKey struct{}

func (s *Server) gate(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CB-Gate")), []byte(s.cfg.GateToken)) != 1 {
			writeErr(w, http.StatusForbidden, "bad_gate_token")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
		h(w, r)
	}
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// ---------- browser API ----------

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"csrfToken": r.Context().Value(csrfKey{}), "version": config.Version, "agents": s.agents()})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	list, err := s.eng.Store().ListConversations(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var body struct{ Agent, Workdir, Text string }
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	id, err := s.eng.StartConversation(r.Context(), body.Agent, body.Workdir, body.Text)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"id": id})
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	d, err := s.eng.Detail(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "not_found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	var body struct{ Text string }
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	err := s.eng.SendMessage(r.Context(), r.PathValue("id"), body.Text)
	switch {
	case errors.Is(err, run.ErrBusy):
		writeErr(w, 409, "busy")
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "not_found")
	case err != nil:
		writeErr(w, 400, err.Error())
	default:
		w.WriteHeader(202)
	}
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	_ = s.eng.Stop(r.Context(), r.PathValue("id"))
	w.WriteHeader(202)
}

func (s *Server) handleDecide(w http.ResponseWriter, r *http.Request) {
	var body struct{ Decision string }
	if err := readJSON(r, &body); err != nil || (body.Decision != "approve" && body.Decision != "deny") {
		writeErr(w, 400, "bad_request")
		return
	}
	if err := s.eng.Decide(r.Context(), r.PathValue("id"), body.Decision == "approve"); err != nil {
		writeErr(w, 409, "not_pending")
		return
	}
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	conns, scanned, err := s.eng.Store().Connections(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cli, mcp := []model.Connection{}, []model.Connection{}
	for _, c := range conns {
		if c.Kind == "mcp" {
			mcp = append(mcp, c)
		} else {
			cli = append(cli, c)
		}
	}
	writeJSON(w, 200, map[string]any{"scannedAt": scanned, "cli": cli, "mcp": mcp})
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if s.scan != nil {
		if err := s.scan(r.Context()); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	s.bus.Publish(bus.Event{Type: "connections"})
	s.handleConnections(w, r)
}

var validRules = map[string]bool{model.RuleIrreversible: true, model.RuleLive: true, model.RuleEvery: true, model.RuleReadOnly: true, model.RuleOff: true}

func (s *Server) handleRule(w http.ResponseWriter, r *http.Request) {
	var body struct{ Rule string }
	if err := readJSON(r, &body); err != nil || !validRules[body.Rule] {
		writeErr(w, 400, "bad_rule")
		return
	}
	id := r.PathValue("id")
	if err := s.eng.Store().SetRule(r.Context(), id, body.Rule); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	conns, _, _ := s.eng.Store().Connections(r.Context())
	for _, c := range conns {
		if c.ID == id {
			s.bus.Publish(bus.Event{Type: "connections"})
			writeJSON(w, 200, c)
			return
		}
	}
	writeErr(w, 404, "not_found")
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "no streaming")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	ch, cancel := s.bus.Subscribe()
	defer cancel()
	fl.Flush()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-tick.C:
			io.WriteString(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if s.web == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>CloudBuddy</title><body style="font-family:system-ui;padding:40px">The CloudBuddy frontend isn't built. Run <code>make web</code>, then restart.</body>`)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if _, err := fs.Stat(s.web, p); err != nil {
		p = "index.html" // client-side routes
	}
	if !strings.HasSuffix(p, ".html") {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	http.ServeFileFS(w, r, s.web, p)
}

// ---------- gate endpoints ----------

func (s *Server) handlePreTool(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID string        `json:"runId"`
		Input run.HookInput `json:"input"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	writeJSON(w, 200, s.eng.ClaudePreTool(r.Context(), body.RunID, body.Input))
}

func (s *Server) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID string `json:"runId"`
	}
	_ = readJSON(r, &body)
	s.eng.ClaudeSessionStart(body.RunID)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	var body struct{ Token, Cwd string }
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	info, err := s.eng.Release(r.Context(), body.Token, body.Cwd)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, info)
}

func (s *Server) handleExecFinished(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token     string
		ExitCode  int
		Leftovers int
	}
	_ = readJSON(r, &body)
	s.eng.ExecFinished(body.Token, body.ExitCode, body.Leftovers)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

func (s *Server) handleMCPTools(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID  string        `json:"runId"`
		Server string        `json:"server"`
		Tools  []run.MCPTool `json:"tools"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	s.eng.MCPTools(body.RunID, body.Server, body.Tools)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

func (s *Server) handleMCPCall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID  string         `json:"runId"`
		Server string         `json:"server"`
		Tool   string         `json:"tool"`
		Args   map[string]any `json:"args"`
		Meta   map[string]any `json:"meta"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	ok, reason, key := s.eng.MCPCall(r.Context(), body.RunID, body.Server, body.Tool, body.Args, body.Meta)
	d := "deny"
	if ok {
		d = "allow"
	}
	writeJSON(w, 200, map[string]string{"decision": d, "reason": reason, "key": key})
}

func (s *Server) handleMCPResult(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID   string `json:"runId"`
		Key     string `json:"key"`
		Output  string `json:"output"`
		IsError bool   `json:"isError"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	s.eng.MCPResult(r.Context(), body.RunID, body.Key, body.Output, body.IsError)
	writeJSON(w, 200, map[string]string{"ok": "true"})
}
