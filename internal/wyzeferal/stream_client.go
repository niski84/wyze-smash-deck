package wyzeferal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5" // #nosec G401 - required by Wyze web API protocol
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	wyzeStreamEndpoint  = "https://app.wyzecam.com/app/v4/camera/get-streams"
	wyzeRefreshEndpoint = "https://services.wyze.com/api/v2/santa/refresh?tag=webview"

	// Secret key used to compute signature2 for the v4Event API group.
	// Source: Wyze web app JS bundle, module 59807 (pg function).
	wyzeV4EventSecret = "gbJojEBViLklgwyyDikx5ztSvKBXI5oU"

	wyzeAppID   = "strv_e7f78e9e7738dc50"
	wyzeAppInfo = "wyze_web_2.3.1"

	// Wyze doorbell device model — used in get-streams request.
	wyzeDoorbellDeviceModel = "GW_DBD"
)

// ICEServer is one ICE/TURN/STUN server entry returned by get-streams.
type ICEServer struct {
	URL        string `json:"url"`
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
}

// StreamParams holds the WebRTC signaling params for one camera session.
type StreamParams struct {
	SignalingURL string
	ICEServers   []ICEServer
	DeviceID     string
	CapturedAt   time.Time
}

// WyzeStreamClient fetches WebRTC stream params for the Wyze doorbell without
// a browser. Auth flow:
//  1. GET services.wyze.com/api/v2/santa/refresh with the Flask session cookie
//     → fresh OAuth2 JWT (2-hour TTL)
//  2. POST app.wyzecam.com/app/v4/camera/get-streams with JWT +
//     HMAC_MD5(body, MD5(jwt + secret))
type WyzeStreamClient struct {
	mu            sync.Mutex
	sessionCookie string // services.wyze.com Flask session cookie
	accessToken   string // current OAuth2 JWT

	tokenFetchedAt time.Time
	tokenSaved     func(jwt string) // persists refreshed token to settings

	client *http.Client
}

func NewWyzeStreamClient(sessionCookie, cachedJWT string, tokenSaved func(string)) *WyzeStreamClient {
	return &WyzeStreamClient{
		sessionCookie: strings.TrimSpace(sessionCookie),
		accessToken:   strings.TrimSpace(cachedJWT),
		tokenSaved:    tokenSaved,
		client:        &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *WyzeStreamClient) IsConfigured() bool {
	return c.sessionCookie != ""
}

// ensureJWT returns a valid JWT, refreshing via the services.wyze.com session if needed.
func (c *WyzeStreamClient) ensureJWT(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Refresh when no token or approaching expiry (refresh 10 min before 2-hour TTL)
	if c.accessToken == "" || time.Since(c.tokenFetchedAt) > 110*time.Minute {
		jwt, err := c.fetchJWT(ctx)
		if err != nil {
			if c.accessToken != "" {
				log.Printf("[stream-client] JWT refresh failed (%v), using cached token", err)
				return c.accessToken, nil
			}
			return "", fmt.Errorf("JWT refresh: %w", err)
		}
		c.accessToken = jwt
		c.tokenFetchedAt = time.Now()
		if c.tokenSaved != nil {
			c.tokenSaved(jwt)
		}
		log.Printf("[stream-client] JWT refreshed via services.wyze.com")
	}
	return c.accessToken, nil
}

func (c *WyzeStreamClient) fetchJWT(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wyzeRefreshEndpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", "session="+c.sessionCookie)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", wyzeRefreshEndpoint, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("refresh returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var result struct {
		Code        any    `json:"code"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("parse refresh response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("no access_token in refresh response: %s", string(raw)[:200])
	}
	return result.AccessToken, nil
}

// computeSignature2 computes the HMAC-MD5 request signature required by
// the Wyze v4Event API group.
// Formula (from Wyze web app JS module 59807, function pg):
//
//	intermediate = MD5(accessToken + wyzeV4EventSecret)
//	signature2   = HMAC-MD5(jsonBody, intermediate)
func computeSignature2(accessToken, jsonBody string) string {
	// #nosec G401 - MD5 required by Wyze protocol, not a security choice
	mid := md5.Sum([]byte(accessToken + wyzeV4EventSecret))
	midHex := hex.EncodeToString(mid[:])
	mac := hmac.New(md5.New, []byte(midHex)) // #nosec G401
	mac.Write([]byte(jsonBody))
	return hex.EncodeToString(mac.Sum(nil))
}

// GetStreamParams calls the Wyze get-streams API and returns fresh WebRTC params.
func (c *WyzeStreamClient) GetStreamParams(ctx context.Context, deviceID, deviceModel string) (*StreamParams, error) {
	jwt, err := c.ensureJWT(ctx)
	if err != nil {
		return nil, fmt.Errorf("get JWT: %w", err)
	}

	nonce := time.Now().UnixMilli()
	reqBody := map[string]any{
		"device_list": []map[string]any{
			{
				"device_id":    deviceID,
				"device_model": deviceModel,
				"provider":     "webrtc",
				"parameters":   map[string]any{"use_trickle": true},
			},
		},
		"nonce": nonce,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	bodyStr := string(bodyBytes)
	sig := computeSignature2(jwt, bodyStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wyzeStreamEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", jwt)
	req.Header.Set("access_token", jwt)
	req.Header.Set("appid", wyzeAppID)
	req.Header.Set("appinfo", wyzeAppInfo)
	req.Header.Set("signature2", sig)
	req.Header.Set("requestid", fmt.Sprintf("%d", rand.Intn(100))) // #nosec G404

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST get-streams: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		snippet := string(raw)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("get-streams returned HTTP %d: %s", resp.StatusCode, snippet)
	}

	var result struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			DeviceID string `json:"device_id"`
			Params   struct {
				SignalingURL string      `json:"signaling_url"`
				ICEServers   []ICEServer `json:"ice_servers"`
			} `json:"params"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse get-streams response: %w", err)
	}
	if result.Code != "1" {
		// JWT may be stale even within the 2-hour window — force refresh on next call
		c.mu.Lock()
		c.accessToken = ""
		c.mu.Unlock()
		return nil, fmt.Errorf("get-streams error code=%s msg=%s", result.Code, result.Msg)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("get-streams returned no devices")
	}
	dev := result.Data[0]
	return &StreamParams{
		SignalingURL: dev.Params.SignalingURL,
		ICEServers:   dev.Params.ICEServers,
		DeviceID:     dev.DeviceID,
		CapturedAt:   time.Now(),
	}, nil
}
