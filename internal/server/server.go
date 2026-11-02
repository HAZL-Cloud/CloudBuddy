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
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"cloudbuddy/internal/bus"
	"cloudbuddy/internal/config"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/picker"
	"cloudbuddy/internal/run"
	"cloudbuddy/internal/store"
)

type Agent struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Version    string `json:"version"`
	Available  bool   `json:"available"`
	ComingSoon bool   `json:"comingSoon,omitempty"` // shown, not usable yet (D-066)
}

type Server struct {
	cfg     config.Config
	eng     *run.Engine
	bus     *bus.Bus
	web     fs.FS // built frontend; nil shows a "not built" page
	agents  func() []Agent
	scan    func(ctx context.Context) error
	devPort int
	// pick opens the computer's folder chooser (D-058); tests replace it.
	pick    func(ctx context.Context, prompt, start string) (string, error)
	picking atomic.Bool
}

func New(cfg config.Config, eng *run.Engine, b *bus.Bus, web fs.FS, agents func() []Agent, scan func(context.Context) error) *Server {
	s := &Server{cfg: cfg, eng: eng, bus: b, web: web, agents: agents, scan: scan, pick: picker.Folder}
	if cfg.Dev {
		s.devPort = 5173
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// browser
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("GET /api/conversations", s.handleList)
	mux.HandleFunc("POST /api/conversations", s.handleCreate)
	mux.HandleFunc("GET /api/conversations/{id}", s.handleDetail)
	mux.HandleFunc("POST /api/conversations/{id}/messages", s.handleMessage)
	mux.HandleFunc("POST /api/conversations/{id}/stop", s.handleStop)
	mux.HandleFunc("POST /api/approvals/{id}", s.handleDecide)
	mux.HandleFunc("POST /api/steps/{id}/stop", s.handleStopStep)
	mux.HandleFunc("GET /api/settings", s.handleSettings)
	mux.HandleFunc("PUT /api/settings", s.handleSaveSettings)
	mux.HandleFunc("POST /api/pick-folder", s.handlePickFolder)
	mux.HandleFunc("GET /api/projects", s.handleProjects)
	mux.HandleFunc("POST /api/projects", s.handleSaveProject)
	mux.HandleFunc("PUT /api/projects/{id}", s.handleSaveProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)
	mux.HandleFunc("GET /api/connections", s.handleConnections)
	mux.HandleFunc("POST /api/connections/scan", s.handleScan)
	mux.HandleFunc("PUT /api/connections/{id}/rule", s.handleRule)
	mux.HandleFunc("GET /api/dashboards", s.handleDashboards)
	mux.HandleFunc("GET /api/dashboards/{id}", s.handleDashboard)
	mux.HandleFunc("PUT /api/dashboards/{id}/schedule", s.handleDashboardSchedule)
	mux.HandleFunc("POST /api/dashboards/{id}/refresh", s.handleDashboardRefresh)
	mux.HandleFunc("POST /api/dashboards/{id}/sources/{key}/decision", s.handleDashboardDecision)
	mux.HandleFunc("DELETE /api/dashboards/{id}", s.handleDeleteDashboard)
	mux.HandleFunc("GET /stream", s.handleStream)
	// gate (hook, exec wrapper, MCP gateway): separate secret, can't approve
	mux.HandleFunc("POST /gate/claude/pretool", s.gate(s.handlePreTool))
	mux.HandleFunc("POST /gate/claude/session", s.gate(s.handleSessionStart))
	mux.HandleFunc("POST /gate/release", s.gate(s.handleRelease))
	mux.HandleFunc("POST /gate/exec/finished", s.gate(s.handleExecFinished))
	mux.HandleFunc("POST /gate/mcp/tools", s.gate(s.handleMCPTools))
	mux.HandleFunc("POST /gate/mcp/call", s.gate(s.handleMCPCall))
	mux.HandleFunc("POST /gate/mcp/result", s.gate(s.handleMCPResult))
	mux.HandleFunc("POST /gate/dashboards/tool", s.gate(s.handleDashboardTool))
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
	writeJSON(w, 200, map[string]any{"version": config.Version, "agents": s.agents()})
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
	var body struct {
		Agent, Workdir, Text string
		ProjectID            string `json:"projectId"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	id, err := s.eng.StartConversation(r.Context(), run.StartInput{Agent: body.Agent, Workdir: body.Workdir, ProjectID: body.ProjectID, Text: body.Text})
	switch {
	case errors.Is(err, run.ErrNoFolder):
		writeErr(w, 400, "no_folder")
	case err != nil:
		writeErr(w, 400, err.Error())
	default:
		writeJSON(w, 200, map[string]string{"id": id})
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	set, err := s.eng.Settings(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, set)
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var body model.Settings
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	set, err := s.eng.SaveSettings(r.Context(), body)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, set)
}

// handlePickFolder opens the computer's own folder chooser and returns the
// folder picked (D-058). One chooser at a time; it closes if the page stops
// waiting. Nothing is saved here: the path is checked again where it is used.
func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Start string `json:"start"`
	}
	_ = readJSON(r, &body) // the body is optional
	start := strings.TrimSpace(body.Start)
	if strings.HasPrefix(start, "~/") {
		start = filepath.Join(s.cfg.Home, start[2:])
	}
	if !filepath.IsAbs(start) {
		start = s.cfg.Home
	}
	if !s.picking.CompareAndSwap(false, true) {
		writeErr(w, 409, "busy")
		return
	}
	defer s.picking.Store(false)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	path, err := s.pick(ctx, "Choose a folder for CloudBuddy", start)
	switch {
	case errors.Is(err, picker.ErrCancelled):
		writeJSON(w, 200, map[string]bool{"cancelled": true})
	case errors.Is(err, picker.ErrUnavailable):
		writeErr(w, 501, "no_picker")
	case err != nil:
		writeErr(w, 500, "the folder chooser failed: "+err.Error())
	default:
		writeJSON(w, 200, map[string]string{"path": path})
	}
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	list, err := s.eng.Projects(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

// handleSaveProject creates (POST) or updates (PUT) a project.
func (s *Server) handleSaveProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string   `json:"name"`
		Folders []string `json:"folders"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	p, err := s.eng.SaveProject(r.Context(), model.Project{ID: r.PathValue("id"), Name: body.Name, Folders: body.Folders})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "not_found")
	case err != nil:
		writeErr(w, 400, err.Error())
	default:
		writeJSON(w, 200, p)
	}
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	err := s.eng.DeleteProject(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "not_found")
	case err != nil:
		writeErr(w, 500, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
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
	case errors.Is(err, run.ErrCodexSoon):
		writeErr(w, 409, err.Error())
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

func (s *Server) handleStopStep(w http.ResponseWriter, r *http.Request) {
	if err := s.eng.StopStep(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, 409, "not_running")
		return
	}
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
	var body struct {
		Token, Cwd string
		Pid        int
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	info, err := s.eng.Release(r.Context(), body.Token, body.Cwd, body.Pid)
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
		Output    string
	}
	_ = readJSON(r, &body)
	s.eng.ExecFinished(body.Token, body.ExitCode, body.Leftovers, body.Output)
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

// ShimHandler serves the CLI shims on CloudBuddy's local socket (D-065). The
// shims run inside commands, where CloudBuddy's port is off-limits; each
// request carries the token of the command it came from, which can only add
// a card to that command and wait for the user's answer, never approve.
func (s *Server) ShimHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /shim/start", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token, Prog, Cwd, Copy string
			Args                   []string
		}
		if err := readJSON(r, &body); err != nil || body.Token == "" || body.Prog == "" {
			writeErr(w, 400, "bad_request")
			return
		}
		ok, reason, key := s.eng.ShimStart(r.Context(), body.Token, body.Prog, body.Args, body.Cwd, body.Copy)
		decision := "deny"
		if ok {
			decision = "allow"
		}
		writeJSON(w, 200, map[string]string{"decision": decision, "reason": reason, "key": key})
	})
	mux.HandleFunc("POST /shim/end", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token, Key, Output string
			ExitCode           int
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, 400, "bad_request")
			return
		}
		s.eng.ShimEnd(body.Token, body.Key, body.ExitCode, body.Output)
		writeJSON(w, 200, map[string]string{"ok": "true"})
	})
	return mux
}
