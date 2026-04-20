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
	socketPath  string
	listener    net.Listener
	tcpListener net.Listener
	server      *http.Server
	mux         *http.ServeMux
	engine      *Engine
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

	tcpL, err := net.Listen("tcp", ":8080")
	if err != nil {
		_ = l.Close()
		_ = os.Remove(sockPath)
		return nil, err
	}

	s := &APIServer{
		socketPath:  sockPath,
		listener:    l,
		tcpListener: tcpL,
		mux:         http.NewServeMux(),
		engine:      engine,
	}

	// Register Handlers
	s.mux.HandleFunc("/api/v1/send", s.handleSend)
	s.mux.HandleFunc("/api/v1/ui/manifest", s.handleUIManifest)
	s.mux.HandleFunc("/api/v1/status", s.handleStatus)
	s.mux.HandleFunc("/api/v1/dialogs", s.handleDialogList)
	s.mux.HandleFunc("/api/v1/dialogs/auth/start", s.handleDialogAuthStart)
	s.mux.HandleFunc("/api/v1/dialogs/auth/poll", s.handleDialogAuthPoll)
	s.mux.HandleFunc("/api/v1/dialogs/add", s.handleDialogAdd)
	// Plugin lifecycle management
	s.mux.HandleFunc("/api/v1/plugins", s.handlePluginList)
	s.mux.HandleFunc("/api/v1/plugins/enable", s.handlePluginEnable)
	s.mux.HandleFunc("/api/v1/plugins/disable", s.handlePluginDisable)
	s.mux.HandleFunc("/api/v1/plugins/instances", s.handlePluginInstances)

	// Static Assets (Embedded Core Shell)
	staticFS := GetStaticFS()
	if staticFS != nil {
		s.mux.Handle("/", s.spaHandler(http.FS(staticFS)))
		slog.Info("api: serving embedded static assets")
	}

	return s, nil
}

func (s *APIServer) Start() {
	s.server = &http.Server{Handler: s.mux}

	// Start Unix Socket Server
	go func() {
		slog.Info("api: unix server starting", "path", s.socketPath)
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			slog.Error("api: unix serve failed", "error", err)
		}
	}()

	// Start HTTP TCP Server (for Web UI)
	go func() {
		slog.Info("api: http server starting", "addr", s.tcpListener.Addr().String())
		if err := s.server.Serve(s.tcpListener); err != nil && err != http.ErrServerClosed {
			slog.Error("api: http serve failed", "error", err)
		}
	}()
}

// spaHandler returns a handler that serves index.html for unknown routes.
func (s *APIServer) spaHandler(root http.FileSystem) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only handle GET requests for SPA routing
		if r.Method != http.MethodGet {
			http.FileServer(root).ServeHTTP(w, r)
			return
		}

		// Try to open the file
		f, err := root.Open(r.URL.Path)
		if err != nil {
			// If file not found, serve index.html
			r.URL.Path = "/index.html"
		} else {
			_ = f.Close()
		}

		http.FileServer(root).ServeHTTP(w, r)
	})
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

func (s *APIServer) handleUIManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Support dev dev
	manifest := s.engine.GetUIManifest()
	_ = json.NewEncoder(w).Encode(manifest)
}

func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	status := s.engine.GetStatus(r.Context())
	_ = json.NewEncoder(w).Encode(status)
}

func (s *APIServer) handleDialogList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	instances := s.engine.GetDialogInstances()
	_ = json.NewEncoder(w).Encode(instances)
}

func (s *APIServer) handleDialogAuthStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		http.Error(w, "missing platform", http.StatusBadRequest)
		return
	}
	sess, err := s.engine.StartDialogAuth(r.Context(), platform)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(sess)
}

func (s *APIServer) handleDialogAuthPoll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	platform := r.URL.Query().Get("platform")
	sessionID := r.URL.Query().Get("session_id")
	if platform == "" || sessionID == "" {
		http.Error(w, "missing platform or session_id", http.StatusBadRequest)
		return
	}
	sess, err := s.engine.PollDialogAuth(r.Context(), platform, sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(sess)
}

func (s *APIServer) handleDialogAdd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Platform string            `json:"platform"`
		Params   map[string]string `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.engine.AddDialogInstance(r.Context(), req.Platform, req.Params); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// PluginInfo is the API representation of an available plugin type.
type PluginInfo struct {
	Name        string        `json:"name"`
	Category    string        `json:"category"` // "dialog", "llm", "skill"
	Description string        `json:"description"`
	AuthType    string        `json:"auth_type,omitempty"` // "", "qr", "token"
	Fields      []ConfigField `json:"fields"`
	Enabled     bool          `json:"enabled"`
	Instances   []any         `json:"instances,omitempty"` // active bot accounts when enabled
}

// handlePluginList returns all registered plugin factories with their current enabled state.
func (s *APIServer) handlePluginList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	s.engine.mu.RLock()
	enabledDialogs := make(map[string]bool, len(s.engine.dialogs))
	for name := range s.engine.dialogs {
		enabledDialogs[name] = true
	}
	s.engine.mu.RUnlock()

	// Retrieve active instance details for enabled dialogs
	instanceMap := s.engine.GetDialogInstances()

	var plugins []PluginInfo //nolint:prealloc // length is not known ahead of iteration
	for _, name := range ListDialogFactories() {
		spec, _ := GetPluginConfigSpec(name)
		info := PluginInfo{
			Name:        name,
			Category:    "dialog",
			Description: spec.Description,
			AuthType:    string(spec.AuthType),
			Fields:      spec.Fields,
			Enabled:     enabledDialogs[name],
		}
		if insts, ok := instanceMap[name]; ok {
			raw := make([]any, len(insts))
			for i, v := range insts {
				raw[i] = v
			}
			info.Instances = raw
		}
		plugins = append(plugins, info)
	}

	_ = json.NewEncoder(w).Encode(plugins)
}

// handlePluginInstances returns all persisted plugin instances from the store.
func (s *APIServer) handlePluginInstances(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	instances, err := s.engine.GetPluginInstances()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if instances == nil {
		instances = []PluginInstance{}
	}
	_ = json.NewEncoder(w).Encode(instances)
}

// handlePluginEnable creates and starts a dialog plugin with the provided config.
func (s *APIServer) handlePluginEnable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name   string         `json:"name"`
		Config map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "missing plugin name", http.StatusBadRequest)
		return
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}

	if err := s.engine.EnablePlugin(req.Name, req.Config); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "name": req.Name})
}

// handlePluginDisable stops a running dialog plugin and marks it disabled.
func (s *APIServer) handlePluginDisable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "missing plugin name", http.StatusBadRequest)
		return
	}

	if err := s.engine.DisablePlugin(req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "name": req.Name})
}
