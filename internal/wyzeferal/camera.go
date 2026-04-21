package wyzeferal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// snapshotCache holds the last proxied camera snapshot to avoid hitting the
// Wyze CDN on every browser poll. It also caches the signed URL with a short
// TTL — on CDN 401 we clear the URL so the next fetch gets a fresh one from
// the Wyze API.
type snapshotCache struct {
	mu          sync.Mutex
	data        []byte
	contentType string
	url         string  // signed CDN URL; cleared on 401
	urlFetched  time.Time
	imgFetched  time.Time
}

const (
	snapshotImgTTL = 8 * time.Second
	snapshotURLTTL = 4 * time.Minute // signed URLs typically live ~15 min
)

func (sc *snapshotCache) getImg() ([]byte, string, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.data == nil || time.Since(sc.imgFetched) > snapshotImgTTL {
		return nil, "", false
	}
	return sc.data, sc.contentType, true
}

func (sc *snapshotCache) getCachedURL() string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.url == "" || time.Since(sc.urlFetched) > snapshotURLTTL {
		return ""
	}
	return sc.url
}

func (sc *snapshotCache) setURL(url string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.url = url
	sc.urlFetched = time.Now()
}

func (sc *snapshotCache) clearURL() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.url = ""
}

func (sc *snapshotCache) setImg(ct string, data []byte) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.contentType = ct
	sc.data = data
	sc.imgFetched = time.Now()
}

// handleCameraSnapshot proxies the latest Wyze doorbell thumbnail.
// GET /api/camera/snapshot?name=doorbell
func (s *HTTPServer) handleCameraSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
	if name == "" {
		name = "doorbell"
	}

	// Serve cached image if still fresh
	if data, ct, ok := s.snapCache.getImg(); ok {
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}

	// Get (or refresh) the signed CDN URL
	thumbURL := s.snapCache.getCachedURL()
	if thumbURL == "" {
		thumbURL = s.fetchThumbnailURL(r.Context(), name)
		if thumbURL == "" {
			http.Error(w, "no thumbnail available for camera "+name, http.StatusNotFound)
			return
		}
		s.snapCache.setURL(thumbURL)
	}

	data, ct, err := s.fetchFromCDN(thumbURL)
	if err != nil && strings.Contains(err.Error(), "401") {
		// Signed URL expired — clear cached URL and try a fresh one once
		s.snapCache.clearURL()
		thumbURL = s.fetchThumbnailURL(r.Context(), name)
		if thumbURL != "" {
			s.snapCache.setURL(thumbURL)
			data, ct, err = s.fetchFromCDN(thumbURL)
		}
	}
	if err != nil {
		// Wyze CDN blocks server-side fetches — return a placeholder SVG so the
		// UI renders gracefully rather than a broken image icon.
		serveCameraPlaceholder(w, name)
		return
	}

	s.snapCache.setImg(ct, data)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// fetchThumbnailURL calls the Wyze API (bypassing device cache) to get a fresh
// signed thumbnail URL for the named camera.
func (s *HTTPServer) fetchThumbnailURL(ctx context.Context, name string) string {
	// Bypass the server-side device cache — we need a fresh signed URL
	client := s.wyzeClient()
	devices, err := client.ListDevices(ctx)
	if err != nil {
		log.Printf("[camera] ListDevices error: %v", err)
		return ""
	}
	for _, d := range devices {
		if strings.EqualFold(d.Nickname, name) {
			return d.ThumbnailURL
		}
	}
	return ""
}

func (s *HTTPServer) fetchFromCDN(url string) ([]byte, string, error) {
	resp, err := http.Get(url) // #nosec G107 — URL from Wyze API
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, "", fmt.Errorf("401 CDN signature expired")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("CDN returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	return data, ct, nil
}

// handleCameraInfo returns camera online status and last-seen timestamp.
// GET /api/camera/info?name=doorbell
func (s *HTTPServer) handleCameraInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
	if name == "" {
		name = "doorbell"
	}
	client := s.wyzeClient()
	devices, err := client.ListDevices(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiResp{Success: false, Error: err.Error()})
		return
	}
	for _, d := range devices {
		if strings.EqualFold(d.Nickname, name) {
			writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{
				"nickname":      d.Nickname,
				"model":         d.Model,
				"online":        d.IsOnline,
				"thumbnail_ts":  d.ThumbnailTS,
				"has_thumbnail": d.ThumbnailURL != "",
			}})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, apiResp{Success: false, Error: "camera not found: " + name})
}

// handleWebhookUnifi receives UniFi Protect motion/line-crossing webhooks.
// POST /api/webhooks/unifi
func (s *HTTPServer) handleWebhookUnifi(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResp{Success: false, Error: "read error"})
		return
	}

	// Parse as generic JSON to extract useful fields
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		// Accept even unparseable bodies — still broadcast
		payload = map[string]any{"raw": string(body)}
	}

	log.Printf("[webhook] UniFi event: %s", summarizeWebhook(payload))

	// Broadcast motion event to all SSE clients
	s.sseHub.Broadcast(SSEEvent{
		Type:    "motion",
		Payload: payload,
	})

	writeJSON(w, http.StatusOK, apiResp{Success: true})
}

// handleCameraStreamToken returns fresh WebRTC stream parameters.
// The background streamRefresher keeps these up-to-date every 3 minutes.
// GET /api/camera/stream-token
func (s *HTTPServer) handleCameraStreamToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResp{Success: false, Error: "method not allowed"})
		return
	}

	if s.streamRefresher == nil {
		writeJSON(w, http.StatusServiceUnavailable, apiResp{
			Success: false, Error: "stream client not configured — set wyze_web_session_cookie in settings",
		})
		return
	}

	p := s.streamRefresher.get()
	if p == nil {
		// Nothing cached yet — try a synchronous fetch (first startup)
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if err := s.streamRefresher.refresh(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, apiResp{
				Success: false, Error: "stream params not yet available: " + err.Error(),
			})
			return
		}
		p = s.streamRefresher.get()
	}

	if p == nil || time.Since(p.CapturedAt) > streamStaleCutoff {
		writeJSON(w, http.StatusServiceUnavailable, apiResp{
			Success: false, Error: "stream params stale — refresher may be failing",
		})
		return
	}

	iceList := make([]map[string]any, len(p.ICEServers))
	for i, ice := range p.ICEServers {
		iceList[i] = map[string]any{"url": ice.URL, "username": ice.Username, "credential": ice.Credential}
	}
	writeJSON(w, http.StatusOK, apiResp{Success: true, Data: map[string]any{
		"signaling_url": p.SignalingURL,
		"ice_servers":   iceList,
		"device_id":     p.DeviceID,
	}})
}

func serveCameraPlaceholder(w http.ResponseWriter, name string) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360">
  <rect width="640" height="360" fill="#0d101a"/>
  <rect x="220" y="120" width="200" height="120" rx="12" fill="#1a1e30" stroke="#2a324d" stroke-width="2"/>
  <circle cx="320" cy="180" r="36" fill="#1a1e30" stroke="#3b4a78" stroke-width="3"/>
  <circle cx="320" cy="180" r="22" fill="#2a3560"/>
  <circle cx="310" cy="170" r="6" fill="#4a6fa5" opacity="0.6"/>
  <rect x="222" y="108" width="60" height="16" rx="8" fill="#1a1e30" stroke="#2a324d" stroke-width="2"/>
  <circle cx="298" cy="134" r="5" fill="#d41919"/>
  <text x="320" y="276" text-anchor="middle" font-family="monospace" font-size="14" fill="#4a5568">` + name + `</text>
  <text x="320" y="300" text-anchor="middle" font-family="monospace" font-size="11" fill="#3b4a78">stream unavailable · webhook alerts active</text>
</svg>`
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, svg)
}

func summarizeWebhook(p map[string]any) string {
	parts := []string{}
	for _, k := range []string{"type", "smartDetectTypes", "camera", "score"} {
		if v := p[k]; v != nil {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%v", p)
	}
	return strings.Join(parts, " ")
}
