package wyzeferal

import (
	"bytes"
	"context"
	"crypto/md5" // #nosec G401 - required by Wyze protocol
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	wyzeAuthBase   = "https://auth-prod.api.wyze.com"
	wyzeAppBase    = "https://api.wyzecam.com"
	wyzeSC         = "9f275790cab94a72bd206c8876429f3c"
	wyzeSV         = "9d74946e652647e9b6c9d59326aef104"
	wyzeAppVersion = "2.18.43"
	wyzeAppVer     = "com.hualai.WyzeCam___2.18.43"
	wyzeAppName    = "com.hualai.WyzeCam"
	wyzePhoneID    = "go-wyze-smash-deck"
	wyzePublicKey  = "WMXHYf79Nr5gIlt3r0r7p9Tcw5bvs6BB4U8O8nGJ"
)

// WyzeClient interacts with Wyze APIs using email/password + developer API key.
// Auth flow: POST /api/user/login → access_token → POST /app/v2/home_page/get_object_list
type WyzeClient struct {
	email        string
	password     string
	keyID        string
	apiKey       string
	accessToken  string
	refreshToken string
	client       *http.Client
	// tokenSaved is called whenever login/refresh produces new tokens, so the
	// caller can persist them without the client needing file I/O.
	tokenSaved func(access, refresh string)
}

// WyzeDevice represents a device returned by the Wyze API.
type WyzeDevice struct {
	MAC          string `json:"mac"`
	DeviceMAC    string `json:"device_mac"`
	Model        string `json:"product_model"`
	Nickname     string `json:"nickname"`
	Type         string `json:"product_type"`
	IsOnline     bool   `json:"is_online"`
	SwitchState  int    `json:"switch_state"`
	DeviceParams struct {
		SwitchState int `json:"switch_state"`
	} `json:"device_params"`
}

// EffectiveMAC returns the device's stable identifier from whichever field the API populated.
func (d *WyzeDevice) EffectiveMAC() string {
	if s := strings.TrimSpace(d.MAC); s != "" {
		return s
	}
	return strings.TrimSpace(d.DeviceMAC)
}

// EffectiveSwitchState returns switch state from device_params if set, else top-level.
func (d *WyzeDevice) EffectiveSwitchState() int {
	if d.DeviceParams.SwitchState != 0 || d.SwitchState == 0 {
		return d.DeviceParams.SwitchState
	}
	return d.SwitchState
}

// IsOn returns true if power state is on.
func (d *WyzeDevice) IsOn() bool {
	return d.EffectiveSwitchState() == 1
}

// NewWyzeClient creates a client. accessToken / refreshToken are optional cached credentials.
func NewWyzeClient(email, password, keyID, apiKey, accessToken, refreshToken string, tokenSaved func(string, string)) *WyzeClient {
	return &WyzeClient{
		email:        strings.TrimSpace(email),
		password:     strings.TrimSpace(password),
		keyID:        strings.TrimSpace(keyID),
		apiKey:       strings.TrimSpace(apiKey),
		accessToken:  strings.TrimSpace(accessToken),
		refreshToken: strings.TrimSpace(refreshToken),
		client:       &http.Client{Timeout: 20 * time.Second},
		tokenSaved:   tokenSaved,
	}
}

// IsConfigured returns true when the minimum credentials are present.
func (c *WyzeClient) IsConfigured() bool {
	return c.email != "" && c.password != "" && c.keyID != "" && c.apiKey != ""
}

// hashPassword hashes password as md5(md5(md5(input))) per Wyze protocol.
func hashPassword(pw string) string {
	h1 := md5.Sum([]byte(pw)) // #nosec G401
	h2 := md5.Sum([]byte(hex.EncodeToString(h1[:])))
	h3 := md5.Sum([]byte(hex.EncodeToString(h2[:])))
	return hex.EncodeToString(h3[:])
}

// login performs a full email/password login and updates access/refresh tokens.
func (c *WyzeClient) login(ctx context.Context) error {
	if c.email == "" || c.password == "" {
		return fmt.Errorf("wyze email/password not configured")
	}
	payload := map[string]any{
		"email":    c.email,
		"password": hashPassword(c.password),
	}
	headers := map[string]string{
		"keyid":      c.keyID,
		"apikey":     c.apiKey,
		"User-Agent": "wyzeapy",
	}
	resp, err := c.postJSON(ctx, wyzeAuthBase+"/api/user/login", payload, headers)
	if err != nil {
		return fmt.Errorf("wyze login: %w", err)
	}
	access := stringVal(resp, "access_token")
	refresh := stringVal(resp, "refresh_token")
	if access == "" {
		return fmt.Errorf("wyze login: no access_token in response (check email/password)")
	}
	c.accessToken = access
	c.refreshToken = refresh
	if c.tokenSaved != nil {
		c.tokenSaved(access, refresh)
	}
	return nil
}

// refreshAccess exchanges a refresh_token for a new access_token.
func (c *WyzeClient) refreshAccess(ctx context.Context) error {
	if c.refreshToken == "" {
		return fmt.Errorf("no refresh token; re-login required")
	}
	payload := map[string]any{
		"phone_system_type": "1",
		"app_version":       wyzeAppVersion,
		"app_ver":           wyzeAppVer,
		"sc":                wyzeSC,
		"sv":                wyzeSV,
		"phone_id":          wyzePhoneID,
		"app_name":          wyzeAppName,
		"ts":                time.Now().Unix(),
		"refresh_token":     c.refreshToken,
	}
	headers := map[string]string{"X-API-Key": wyzePublicKey}
	resp, err := c.postJSON(ctx, wyzeAppBase+"/app/user/refresh_token", payload, headers)
	if err != nil {
		return fmt.Errorf("wyze refresh: %w", err)
	}
	if code := stringVal(resp, "code"); code != "" && code != "1" {
		return fmt.Errorf("wyze refresh error code=%s msg=%s", code, stringVal(resp, "msg"))
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		return fmt.Errorf("wyze refresh: unexpected response shape")
	}
	access := stringVal(data, "access_token")
	refresh := stringVal(data, "refresh_token")
	if access == "" {
		return fmt.Errorf("wyze refresh: no access_token in response")
	}
	c.accessToken = access
	if refresh != "" {
		c.refreshToken = refresh
	}
	if c.tokenSaved != nil {
		c.tokenSaved(c.accessToken, c.refreshToken)
	}
	return nil
}

// ensureToken returns a valid access token, logging in or refreshing as needed.
func (c *WyzeClient) ensureToken(ctx context.Context) (string, error) {
	if c.accessToken != "" {
		return c.accessToken, nil
	}
	if c.refreshToken != "" {
		if err := c.refreshAccess(ctx); err == nil {
			return c.accessToken, nil
		}
	}
	if err := c.login(ctx); err != nil {
		return "", err
	}
	return c.accessToken, nil
}

// ListDevices returns all Wyze devices on the account.
func (c *WyzeClient) ListDevices(ctx context.Context) ([]WyzeDevice, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("wyze credentials not configured (email, password, key_id, api_key required)")
	}
	token, err := c.ensureToken(ctx)
	if err != nil {
		return nil, err
	}
	devices, err := c.fetchDevices(ctx, token)
	if err != nil && isAuthErr(err) {
		// Try a fresh login and retry once
		if lerr := c.login(ctx); lerr == nil {
			devices, err = c.fetchDevices(ctx, c.accessToken)
		}
	}
	return devices, err
}

func (c *WyzeClient) fetchDevices(ctx context.Context, token string) ([]WyzeDevice, error) {
	payload := map[string]any{
		"phone_system_type": "1",
		"app_version":       wyzeAppVersion,
		"app_ver":           wyzeAppVer,
		"sc":                wyzeSC,
		"sv":                wyzeSV,
		"access_token":      token,
		"phone_id":          wyzePhoneID,
		"app_name":          wyzeAppName,
		"ts":                time.Now().Unix(),
	}
	resp, err := c.postJSON(ctx, wyzeAppBase+"/app/v2/home_page/get_object_list", payload, nil)
	if err != nil {
		return nil, err
	}
	if code := stringVal(resp, "code"); code != "" && code != "1" {
		msg := stringVal(resp, "msg")
		if msg == "" {
			msg = stringVal(resp, "message")
		}
		return nil, fmt.Errorf("wyze api error code=%s: %s", code, msg)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response shape: missing data object")
	}
	devList, _ := data["device_list"].([]any)
	out := make([]WyzeDevice, 0, len(devList))
	for _, raw := range devList {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		d := wyzeDeviceFromMap(m)
		if d.EffectiveMAC() == "" {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// ControlDevice turns a device on or off using the Wyze set_property API.
func (c *WyzeClient) ControlDevice(ctx context.Context, mac, model string, on bool) error {
	if !c.IsConfigured() {
		return fmt.Errorf("wyze credentials not configured")
	}
	token, err := c.ensureToken(ctx)
	if err != nil {
		return err
	}
	err = c.setProperty(ctx, token, mac, model, "P3", boolToStr(on))
	if err != nil && isAuthErr(err) {
		if lerr := c.login(ctx); lerr == nil {
			err = c.setProperty(ctx, c.accessToken, mac, model, "P3", boolToStr(on))
		}
	}
	return err
}

func (c *WyzeClient) setProperty(ctx context.Context, token, mac, model, pid, pvalue string) error {
	payload := map[string]any{
		"phone_system_type": "1",
		"app_version":       wyzeAppVersion,
		"app_ver":           wyzeAppVer,
		"sc":                wyzeSC,
		"sv":                wyzeSV,
		"access_token":      token,
		"phone_id":          wyzePhoneID,
		"app_name":          wyzeAppName,
		"ts":                time.Now().Unix(),
		"device_mac":        mac,
		"device_model":      model,
		"pid":               pid,
		"pvalue":            pvalue,
	}
	resp, err := c.postJSON(ctx, wyzeAppBase+"/app/v2/device/set_property", payload, nil)
	if err != nil {
		return err
	}
	if code := stringVal(resp, "code"); code != "" && code != "1" {
		return fmt.Errorf("wyze set_property error code=%s msg=%s", code, stringVal(resp, "msg"))
	}
	return nil
}

// SetBrightness sets brightness (1-100) on a mesh light using Wyze property P1501.
func (c *WyzeClient) SetBrightness(ctx context.Context, mac, model string, brightness int) error {
	if !c.IsConfigured() {
		return fmt.Errorf("wyze credentials not configured")
	}
	if brightness < 1 {
		brightness = 1
	}
	if brightness > 100 {
		brightness = 100
	}
	token, err := c.ensureToken(ctx)
	if err != nil {
		return err
	}
	// P1501 = brightness (1-100), P1502 = color temp. Use set_property_list for atomicity.
	err = c.setPropertyList(ctx, token, mac, model, []map[string]string{
		{"pid": "P1501", "pvalue": fmt.Sprintf("%d", brightness)},
	})
	if err != nil && isAuthErr(err) {
		if lerr := c.login(ctx); lerr == nil {
			err = c.setPropertyList(ctx, c.accessToken, mac, model, []map[string]string{
				{"pid": "P1501", "pvalue": fmt.Sprintf("%d", brightness)},
			})
		}
	}
	return err
}

// SetColor sets the RGB color of a color-capable Wyze mesh bulb (e.g. HL_BR30C, HL_A19C2).
// hexColor must be "#RRGGBB" or "RRGGBB".
//
// Wyze mesh light color protocol (confirmed via ha-wyze-control-analyzer):
//   P1508 = "1"      — switch bulb into color mode (vs white/temperature mode)
//   P1507 = "RRGGBB" — the hex color string (uppercase, no #)
func (c *WyzeClient) SetColor(ctx context.Context, mac, model, hexColor string) error {
	if !c.IsConfigured() {
		return fmt.Errorf("wyze credentials not configured")
	}
	normalized := normalizeHexColor(hexColor)
	if normalized == "" {
		return fmt.Errorf("invalid hex color %q — expected #RRGGBB", hexColor)
	}
	props := []map[string]string{
		{"pid": "P1508", "pvalue": "1"},        // color mode flag
		{"pid": "P1507", "pvalue": normalized}, // hex color string
	}
	token, err := c.ensureToken(ctx)
	if err != nil {
		return err
	}
	err = c.setPropertyList(ctx, token, mac, model, props)
	if err != nil && isAuthErr(err) {
		if lerr := c.login(ctx); lerr == nil {
			err = c.setPropertyList(ctx, c.accessToken, mac, model, props)
		}
	}
	return err
}

// normalizeHexColor strips "#" and uppercases a 6-digit hex color string.
// Returns "" if the input is not a valid 6-digit hex color.
func normalizeHexColor(v string) string {
	out := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "#")))
	if len(out) != 6 {
		return ""
	}
	for _, ch := range out {
		if (ch < '0' || ch > '9') && (ch < 'A' || ch > 'F') {
			return ""
		}
	}
	return out
}

func (c *WyzeClient) setPropertyList(ctx context.Context, token, mac, model string, props []map[string]string) error {
	payload := map[string]any{
		"phone_system_type": "1",
		"app_version":       wyzeAppVersion,
		"app_ver":           wyzeAppVer,
		"sc":                wyzeSC,
		"sv":                wyzeSV,
		"access_token":      token,
		"phone_id":          wyzePhoneID,
		"app_name":          wyzeAppName,
		"ts":                time.Now().Unix(),
		"device_mac":        mac,
		"device_model":      model,
		"property_list":     props,
	}
	resp, err := c.postJSON(ctx, wyzeAppBase+"/app/v2/device/set_property_list", payload, nil)
	if err != nil {
		return err
	}
	if code := stringVal(resp, "code"); code != "" && code != "1" {
		return fmt.Errorf("wyze set_property_list error code=%s msg=%s", code, stringVal(resp, "msg"))
	}
	return nil
}

// GetDeviceProperty fetches power state (on/off) for a device.
func (c *WyzeClient) GetDeviceProperty(ctx context.Context, mac, model string) (on bool, err error) {
	if !c.IsConfigured() {
		return false, fmt.Errorf("wyze credentials not configured")
	}
	token, err := c.ensureToken(ctx)
	if err != nil {
		return false, err
	}
	return c.getProperty(ctx, token, mac, model)
}

func (c *WyzeClient) getProperty(ctx context.Context, token, mac, model string) (bool, error) {
	payload := map[string]any{
		"phone_system_type": "1",
		"app_version":       wyzeAppVersion,
		"app_ver":           wyzeAppVer,
		"sc":                wyzeSC,
		"sv":                wyzeSV,
		"access_token":      token,
		"phone_id":          wyzePhoneID,
		"app_name":          wyzeAppName,
		"ts":                time.Now().Unix(),
		"device_mac":        mac,
		"device_model":      model,
		"pids":              []string{"P3"},
	}
	resp, err := c.postJSON(ctx, wyzeAppBase+"/app/v2/device/get_property_list", payload, nil)
	if err != nil {
		return false, err
	}
	if code := stringVal(resp, "code"); code != "" && code != "1" {
		return false, fmt.Errorf("wyze get_property error code=%s", code)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		return false, nil
	}
	props, _ := data["property_list"].([]any)
	for _, raw := range props {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if stringVal(m, "pid") == "P3" {
			return stringVal(m, "value") == "1", nil
		}
	}
	return false, nil
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func (c *WyzeClient) postJSON(ctx context.Context, url string, payload map[string]any, extraHeaders map[string]string) (map[string]any, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w (body: %.300s)", err, string(raw))
	}
	return out, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func stringVal(m map[string]any, k string) string {
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func isAuthErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "token") || strings.Contains(msg, "auth") ||
		strings.Contains(msg, "expired") || strings.Contains(msg, "invalid")
}

func boolToStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func wyzeDeviceFromMap(m map[string]any) WyzeDevice {
	d := WyzeDevice{}
	for _, k := range []string{"mac", "device_mac"} {
		if s, _ := m[k].(string); strings.TrimSpace(s) != "" {
			d.MAC = strings.TrimSpace(s)
			break
		}
	}
	if s, _ := m["device_mac"].(string); s != "" {
		d.DeviceMAC = s
	}
	if d.DeviceMAC == "" {
		d.DeviceMAC = d.MAC
	}
	if s, _ := m["product_model"].(string); s != "" {
		d.Model = s
	}
	if s, _ := m["nickname"].(string); s != "" {
		d.Nickname = s
	}
	if s, _ := m["product_type"].(string); s != "" {
		d.Type = s
	}

	// is_online may be bool or numeric
	switch v := m["is_online"].(type) {
	case bool:
		d.IsOnline = v
	case float64:
		d.IsOnline = v != 0
	}

	// switch state from device_params first
	if dp, ok := m["device_params"].(map[string]any); ok {
		for _, k := range []string{"switch-power", "power", "P3", "on", "switch_state"} {
			if v, ok := dp[k]; ok {
				switch t := v.(type) {
				case float64:
					if t == 1 {
						d.DeviceParams.SwitchState = 1
					}
				case string:
					if t == "1" {
						d.DeviceParams.SwitchState = 1
					}
				case bool:
					if t {
						d.DeviceParams.SwitchState = 1
					}
				}
				if d.DeviceParams.SwitchState == 1 {
					break
				}
			}
		}
		d.SwitchState = d.DeviceParams.SwitchState
	}
	return d
}

// wyzeDeviceListData used only in unit tests.
type wyzeDeviceListData struct {
	Total      int          `json:"total"`
	DeviceList []WyzeDevice `json:"device_list"`
}

// parseDeviceListFromData used by unit tests to exercise device list parsing.
func parseDeviceListFromData(data json.RawMessage) ([]WyzeDevice, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var asArray []WyzeDevice
	if err := json.Unmarshal(data, &asArray); err == nil {
		for i := range asArray {
			if asArray[i].MAC == "" && asArray[i].DeviceMAC != "" {
				asArray[i].MAC = asArray[i].DeviceMAC
			}
		}
		return asArray, nil
	}
	var listData wyzeDeviceListData
	if err := json.Unmarshal(data, &listData); err == nil && listData.DeviceList != nil {
		return listData.DeviceList, nil
	}
	return nil, fmt.Errorf("parse device list: unrecognized data shape")
}
