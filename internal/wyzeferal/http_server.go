package wyzeferal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// HTTPServer wires HTTP routes to Wyze + automations.
type HTTPServer struct {
	mu sync.RWMutex
	cfg AppConfig

	settingsPath   string
	automationPath string
	logPath        string

	store     *AutomationStore
	registry  *DeviceRegistry
	logger    *AutomationLogger
	scheduler *AutomationScheduler
}

func NewHTTPServer(cfg AppConfig) *HTTPServer {
	settingsPath := DefaultSettingsPath()
	autoPath := filepath.Clean("data/wyzeferal-automations.json")
	logPath := filepath.Clean("data/wyzeferal.log")
	s := &HTTPServer{
		cfg:            cfg,
		settingsPath:   settingsPath,
		automationPath: autoPath,
		logPath:        logPath,
		store:          NewAutomationStore(autoPath),
		registry:       NewDeviceRegistry(filepath.Clean("data/wyzeferal-devices.json")),
		logger:         NewAutomationLogger(logPath),
	}
	s.scheduler = NewAutomationScheduler(s.store, s.logger, s.wyzeClient)
	return s
}

func (s *HTTPServer) wyzeClient() *WyzeClient {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	return NewWyzeClient(
		cfg.WyzeEmail,
		cfg.WyzePassword,
		cfg.WyzeKeyID,
		cfg.WyzeAPIKey,
		cfg.WyzeAccessToken,
		cfg.WyzeRefreshToken,
		func(access, refresh string) {
			s.mu.Lock()
			s.cfg.WyzeAccessToken = access
			s.cfg.WyzeRefreshToken = refresh
			c := s.cfg
			s.mu.Unlock()
			// Persist new tokens so next restart doesn't require a fresh login
			_ = SaveAppConfig(s.settingsPath, c)
		},
	)
}

func (s *HTTPServer) snapshotCfg() AppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *HTTPServer) replaceCfg(c AppConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = c
}

type apiResp struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Routes returns the HTTP handler for the Smash Deck API + static UI.
func (s *HTTPServer) Routes(webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/devices", s.handleDevices) // register before /api/devices/ prefix
	mux.HandleFunc("/api/devices/", s.handleDeviceSubroutes)
	mux.HandleFunc("/api/automations/", s.handleAutomationSubroutes)
	mux.HandleFunc("/api/automations", s.handleAutomations)
	mux.HandleFunc("/api/logs", s.handleLogs)

	fs := http.FileServer(http.Dir(filepath.Clean(webDir)))
	mux.Handle("/", fs)
	return mux
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]string{"service": "wyze-feral-smash-deck"}})
}

func (s *HTTPServer) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := s.snapshotCfg()
		safe := map[string]string{
			"port":          cfg.Port,
			"wyze_email":    cfg.WyzeEmail,
			"wyze_key_id":   cfg.WyzeKeyID,
			"wyze_api_key":  maskKey(cfg.WyzeAPIKey),
			"wyze_password": maskKey(cfg.WyzePassword),
		}
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: safe})
	case http.MethodPost:
		var body struct {
			Port         string `json:"port"`
			WyzeEmail    string `json:"wyze_email"`
			WyzePassword string `json:"wyze_password"`
			WyzeKeyID    string `json:"wyze_key_id"`
			WyzeAPIKey   string `json:"wyze_api_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "invalid JSON"})
			return
		}
		cur := s.snapshotCfg()
		if body.Port != "" {
			cur.Port = body.Port
		}
		if body.WyzeEmail != "" {
			cur.WyzeEmail = body.WyzeEmail
		}
		if body.WyzeKeyID != "" {
			cur.WyzeKeyID = body.WyzeKeyID
		}
		// Never persist masked placeholders from GET /api/settings
		if body.WyzeAPIKey != "" && !isMaskedWyzeAPIKey(body.WyzeAPIKey) {
			cur.WyzeAPIKey = body.WyzeAPIKey
			// New API key means cached tokens are stale — clear them
			cur.WyzeAccessToken = ""
			cur.WyzeRefreshToken = ""
		}
		if body.WyzePassword != "" && !isMaskedWyzeAPIKey(body.WyzePassword) {
			cur.WyzePassword = body.WyzePassword
			cur.WyzeAccessToken = ""
			cur.WyzeRefreshToken = ""
		}
		if err := SaveAppConfig(s.settingsPath, cur); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResp{Success: false, Error: err.Error()})
			return
		}
		s.replaceCfg(cur)
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]bool{"saved": true}})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
	}
}

func maskKey(k string) string {
	if len(k) < 8 {
		if k == "" {
			return ""
		}
		return "****"
	}
	return k[:4] + "…" + k[len(k)-4:]
}

// isMaskedWyzeAPIKey detects values returned by maskKey() or short placeholders — must not be saved as real keys.
func isMaskedWyzeAPIKey(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(s, "…") { // unicode ellipsis from maskKey
		return true
	}
	if strings.HasPrefix(s, "****") {
		return true
	}
	return false
}

func (s *HTTPServer) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
		return
	}
	c := s.wyzeClient()
	if !c.IsConfigured() {
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{"devices": []any{}, "configured": false}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	all, err := c.ListDevices(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiResp{Success: false, Error: err.Error()})
		return
	}
	var enriched []EnrichedDevice
	for _, d := range all {
		if strings.TrimSpace(d.EffectiveMAC()) == "" {
			continue
		}
		if !IncludeDeviceOnDashboard(d) {
			continue
		}
		meta := s.registry.Get(d.EffectiveMAC())
		enriched = append(enriched, BuildEnrichedDevice(d, meta))
	}
	writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{"devices": enriched, "configured": true}})
}

// handleDeviceSubroutes: POST /api/devices/{mac}/control, PUT /api/devices/{mac}/name
func (s *HTTPServer) handleDeviceSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		writeJSON(w, http.StatusNotFound, apiResp{Success: false, Error: "not found"})
		return
	}
	mac := parts[0]
	seg := parts[1]

	switch seg {
	case "control":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
			return
		}
		var req struct {
			Model string `json:"model"`
			On    bool   `json:"on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "invalid JSON"})
			return
		}
		c := s.wyzeClient()
		if !c.IsConfigured() {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "wyze not configured"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if err := c.ControlDevice(ctx, mac, req.Model, req.On); err != nil {
			s.logger.Warn("Control FAILED mac=%s on=%v model=%q err=%v", mac, req.On, req.Model, err)
			writeJSON(w, http.StatusBadGateway, apiResp{Success: false, Error: err.Error()})
			return
		}
		s.logger.Info("Manual control mac=%s on=%v", mac, req.On)
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]bool{"ok": true}})
	case "brightness":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
			return
		}
		var req struct {
			Model      string `json:"model"`
			Brightness int    `json:"brightness"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "invalid JSON"})
			return
		}
		if req.Brightness < 1 || req.Brightness > 100 {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "brightness must be 1-100"})
			return
		}
		c := s.wyzeClient()
		if !c.IsConfigured() {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "wyze not configured"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if err := c.SetBrightness(ctx, mac, req.Model, req.Brightness); err != nil {
			s.logger.Warn("Brightness FAILED mac=%s val=%d model=%q err=%v", mac, req.Brightness, req.Model, err)
			writeJSON(w, http.StatusBadGateway, apiResp{Success: false, Error: err.Error()})
			return
		}
		s.logger.Info("Brightness mac=%s val=%d", mac, req.Brightness)
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{"brightness": req.Brightness}})
	case "color":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
			return
		}
		var req struct {
			Model string `json:"model"`
			Color string `json:"color"` // "#rrggbb"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "invalid JSON"})
			return
		}
		if len(req.Color) != 7 || req.Color[0] != '#' {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "color must be #RRGGBB"})
			return
		}
		c := s.wyzeClient()
		if !c.IsConfigured() {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "wyze not configured"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if err := c.SetColor(ctx, mac, req.Model, req.Color); err != nil {
			s.logger.Warn("Color FAILED mac=%s color=%s model=%q err=%v", mac, req.Color, req.Model, err)
			writeJSON(w, http.StatusBadGateway, apiResp{Success: false, Error: err.Error()})
			return
		}
		s.logger.Info("Color mac=%s color=%s", mac, req.Color)
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{"color": req.Color}})
	case "name":
		if r.Method != http.MethodPut {
			writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
			return
		}
		var req struct {
			LocalDisplayName string `json:"local_display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "invalid JSON"})
			return
		}
		if err := s.registry.SetDisplayName(mac, req.LocalDisplayName); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: err.Error()})
			return
		}
		s.logger.Info("Local display name mac=%s name=%q", mac, strings.TrimSpace(req.LocalDisplayName))
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{"saved": true, "id": strings.ToUpper(strings.TrimSpace(mac))}})
	case "tags":
		if r.Method != http.MethodPut {
			writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
			return
		}
		var req struct {
			Tags []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "invalid JSON"})
			return
		}
		if err := s.registry.SetTags(mac, req.Tags); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: err.Error()})
			return
		}
		s.logger.Info("Tags mac=%s tags=%v", mac, req.Tags)
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{"saved": true, "tags": req.Tags}})
	default:
		writeJSON(w, http.StatusNotFound, apiResp{Success: false, Error: "not found"})
	}
}

func (s *HTTPServer) handleAutomations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.store.List()
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{"automations": list}})
	case http.MethodPost:
		var a Automation
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "invalid JSON"})
			return
		}
		if err := validateAutomation(a); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: err.Error()})
			return
		}
		if a.ID == "" {
			a.ID = newID()
		}
		a.CreatedAt = time.Now()
		a.UpdatedAt = a.CreatedAt
		if err := s.store.Upsert(a); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResp{Success: false, Error: err.Error()})
			return
		}
		s.scheduler.RefreshNextRunTimes()
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: a})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
	}
}

func (s *HTTPServer) handleAutomationSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/automations/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, apiResp{Success: false, Error: "not found"})
		return
	}
	id := parts[0]
	if len(parts) >= 2 && parts[1] == "run" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			_ = s.scheduler.RunAutomation(ctx, id)
		}()
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]string{"status": "started"}})
		return
	}

	switch r.Method {
	case http.MethodPut:
		var a Automation
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "invalid JSON"})
			return
		}
		if a.ID != id {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "id mismatch"})
			return
		}
		if err := validateAutomation(a); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: err.Error()})
			return
		}
		a.UpdatedAt = time.Now()
		if err := s.store.Upsert(a); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResp{Success: false, Error: err.Error()})
			return
		}
		s.scheduler.RefreshNextRunTimes()
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: a})
	case http.MethodDelete:
		if err := s.store.Delete(id); err != nil {
			writeJSON(w, http.StatusNotFound, apiResp{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]bool{"deleted": true}})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
	}
}

func (s *HTTPServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
		return
	}
	n := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}
	if n <= 0 || n > 5000 {
		n = 200
	}
	lines := s.logger.Tail(n)
	writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{"lines": lines}})
}

func validateAutomation(a Automation) error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("name is required")
	}
	rm := InferRunMode(a)
	if rm == ModeScheduled && a.Schedule == nil {
		return fmt.Errorf("scheduled automations require a schedule")
	}
	if rm == ModeScheduled && a.Type != TypeScheduled && a.Type != TypeScene {
		return fmt.Errorf("only simple or scene workflows can be scheduled (for now)")
	}
	if a.Type == TypeScene && len(a.SceneSteps) == 0 {
		return fmt.Errorf("scene requires at least one step")
	}
	if a.Type == TypeScheduled && len(a.DeviceMACs) == 0 {
		return fmt.Errorf("simple scheduled automation requires devices")
	}
	if a.Type == TypeTimer && len(a.DeviceMACs) == 0 {
		return fmt.Errorf("timer automation requires devices")
	}
	if a.Type == TypeSafety && len(a.DeviceMACs) == 0 {
		return fmt.Errorf("safety automation requires devices")
	}
	return nil
}

// StartScheduler starts the background ticker.
func (s *HTTPServer) StartScheduler() { s.scheduler.Start() }

// StopScheduler stops the scheduler.
func (s *HTTPServer) StopScheduler() { s.scheduler.Stop() }
