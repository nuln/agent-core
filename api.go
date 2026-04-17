package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

// APIServer exposes a local Unix socket API.
type APIServer struct {
	socketPath string
	listener   net.Listener
	server     *http.Server
	mux        *http.ServeMux
	engine     *Engine
}

func NewAPIServer(dataDir string, engine *Engine) (*APIServer, error) {
	sockDir := filepath.Join(dataDir, "run")
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		slog.Debug("api: mkdir failed", "error", err)
	}
	sockPath := filepath.Join(sockDir, "api.sock")
	_ = os.Remove(sockPath) // Remove old socket if exists

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(sockPath, 0o600); err != nil {
		slog.Debug("api: chmod failed", "error", err)
	}

	s := &APIServer{
		socketPath: sockPath,
		listener:   l,
		mux:        http.NewServeMux(),
		engine:     engine,
	}
	s.mux.HandleFunc("/send", s.handleSend)
	return s, nil
}

func (s *APIServer) Start() {
	s.server = &http.Server{Handler: s.mux}
	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			slog.Error("api: serve failed", "error", err)
		}
	}()
}

func (s *APIServer) Stop() {
	if s.server != nil {
		_ = s.server.Close()
	}
	_ = os.Remove(s.socketPath)
}

func (s *APIServer) handleSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionKey string `json:"session_key"`
		Message    string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Simplified: injection logic to be completed in engine
	_, _ = fmt.Fprintf(w, "OK")
}
