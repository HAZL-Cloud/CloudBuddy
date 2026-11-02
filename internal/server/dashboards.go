package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"cloudbuddy/internal/store"
)

// ---------- dashboards (D-067, D-068) ----------

func (s *Server) handleDashboards(w http.ResponseWriter, r *http.Request) {
	list, err := s.eng.Dashboards(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.eng.DashboardDetail(r.Context(), r.PathValue("id"), r.URL.Query().Get("range"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "not_found")
	case err != nil:
		writeErr(w, 500, err.Error())
	default:
		writeJSON(w, 200, d)
	}
}

func (s *Server) handleDashboardSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Refresh string `json:"refresh"`
		Paused  bool   `json:"paused"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	id := r.PathValue("id")
	err := s.eng.SetDashboardSchedule(r.Context(), id, body.Refresh, body.Paused)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "not_found")
	case err != nil:
		writeErr(w, 400, err.Error())
	default:
		s.handleDashboard(w, r)
	}
}

func (s *Server) handleDashboardRefresh(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.eng.Store().GetDashboard(r.Context(), id); err != nil {
		writeErr(w, 404, "not_found")
		return
	}
	s.eng.RefreshDashboardSoon(id)
	w.WriteHeader(202)
}

// handleDashboardDecision is the user's answer for a command that isn't a
// read: allow it on the timer, or not.
func (s *Server) handleDashboardDecision(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision string `json:"decision"`
	}
	if err := readJSON(r, &body); err != nil || (body.Decision != "allow" && body.Decision != "deny") {
		writeErr(w, 400, "bad_request")
		return
	}
	err := s.eng.AllowDashboardSource(r.Context(), r.PathValue("id"), r.PathValue("key"), body.Decision == "allow")
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "not_found")
	case err != nil:
		writeErr(w, 400, err.Error())
	default:
		s.handleDashboard(w, r)
	}
}

func (s *Server) handleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	err := s.eng.DeleteDashboard(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "not_found")
	case err != nil:
		writeErr(w, 500, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleDashboardTool runs one of CloudBuddy's own MCP tools for a chat's
// agent (gate secret: it can save dashboards, never approve anything).
func (s *Server) handleDashboardTool(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID     string          `json:"runId"`
		ToolUseID string          `json:"toolUseId"`
		Tool      string          `json:"tool"`
		Args      json.RawMessage `json:"args"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "bad_request")
		return
	}
	writeJSON(w, 200, s.eng.DashboardTool(r.Context(), body.RunID, body.ToolUseID, body.Tool, body.Args))
}
