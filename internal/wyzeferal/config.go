package wyzeferal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AppConfig is persisted settings for the Smash Deck server.
type AppConfig struct {
	Port             string `json:"port"`
	WyzeEmail        string `json:"wyze_email"`
	WyzePassword     string `json:"wyze_password"`
	WyzeKeyID        string `json:"wyze_key_id"`
	WyzeAPIKey       string `json:"wyze_api_key"`
	WyzeAccessToken  string `json:"wyze_access_token,omitempty"`
	WyzeRefreshToken string `json:"wyze_refresh_token,omitempty"`

	// Web OAuth2 credentials (my.wyze.com) used for WebRTC get-streams.
	// Both session and remember_token cookies are required by services.wyze.com.
	WyzeWebSessionCookie      string `json:"wyze_web_session_cookie,omitempty"`
	WyzeWebRememberToken      string `json:"wyze_web_remember_token,omitempty"`
	WyzeWebAccessToken        string `json:"wyze_web_access_token,omitempty"`
}

func DefaultSettingsPath() string {
	return filepath.Clean("data/wyzeferal-settings.json")
}

func LoadAppConfig(path string) AppConfig {
	cfg := AppConfig{Port: getenv("PORT", getenv("WYZE_FERAL_PORT", "8082"))}
	raw, err := os.ReadFile(path)
	if err == nil {
		var stored AppConfig
		if json.Unmarshal(raw, &stored) == nil {
			if stored.Port != "" {
				cfg.Port = stored.Port
			}
			cfg.WyzeEmail = stored.WyzeEmail
			cfg.WyzePassword = stored.WyzePassword
			cfg.WyzeKeyID = stored.WyzeKeyID
			cfg.WyzeAPIKey = stored.WyzeAPIKey
			cfg.WyzeAccessToken = stored.WyzeAccessToken
			cfg.WyzeRefreshToken = stored.WyzeRefreshToken
			cfg.WyzeWebSessionCookie = stored.WyzeWebSessionCookie
			cfg.WyzeWebRememberToken = stored.WyzeWebRememberToken
			cfg.WyzeWebAccessToken = stored.WyzeWebAccessToken
		}
	}
	// Env always wins for credentials
	if v := strings.TrimSpace(os.Getenv("WYZE_EMAIL")); v != "" {
		cfg.WyzeEmail = v
	}
	if v := strings.TrimSpace(os.Getenv("WYZE_PASSWORD")); v != "" {
		cfg.WyzePassword = v
	}
	if v := strings.TrimSpace(os.Getenv("WYZE_KEY_ID")); v != "" {
		cfg.WyzeKeyID = v
	}
	if v := strings.TrimSpace(os.Getenv("WYZE_API_KEY")); v != "" {
		cfg.WyzeAPIKey = v
	}
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		cfg.Port = p
	}
	return cfg
}

func SaveAppConfig(path string, cfg AppConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
