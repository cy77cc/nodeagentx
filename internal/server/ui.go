package server

import (
	"embed"
	"io/fs"
	"net/http"
	"time"
)

//go:embed ui/*
var uiFS embed.FS

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	subFS, err := fs.Sub(uiFS, "ui")
	if err != nil {
		http.Error(w, "UI not available", http.StatusInternalServerError)
		return
	}
	http.StripPrefix("/ui/", http.FileServer(http.FS(subFS))).ServeHTTP(w, r)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	cfg := map[string]any{
		"auth": map[string]any{
			"enabled": s.GetAuth().Enabled,
		},
		"prometheus": map[string]any{
			"enabled":           s.GetPrometheus().Enabled,
			"path":              s.GetPrometheus().Path,
			"protect_with_auth": s.GetPrometheus().ProtectWithAuth,
		},
	}

	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: cfg})
}

func (s *Server) handleDetailedHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}

	status, subsystems := s.buildHealthData()
	data := map[string]any{
		"status":         status,
		"subsystems":     subsystems,
		"version":        s.version,
		"git_commit":     s.gitCommit,
		"uptime_seconds": int(time.Since(s.startedAt).Seconds()),
	}

	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: data})
}

func (s *Server) handleLogsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send an initial comment to establish the connection.
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()

	ctx := r.Context()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Send a keep-alive comment.
			_, err := w.Write([]byte(": keepalive\n\n"))
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
