package wyzeferal

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DeviceRegistry stores Smash-Deck–local metadata keyed by device ID (Wyze MAC).
// Wyze cloud nicknames can change; we always key by MAC.
type DeviceRegistry struct {
	mu   sync.RWMutex
	path string
	Data map[string]DeviceLocalMeta `json:"devices"`
}

// DeviceLocalMeta is editable in the UI without calling Wyze rename APIs.
type DeviceLocalMeta struct {
	DisplayName string `json:"display_name,omitempty"`
}

func defaultRegistryPath() string {
	return filepath.Clean("data/wyzeferal-devices.json")
}

func NewDeviceRegistry(path string) *DeviceRegistry {
	if path == "" {
		path = defaultRegistryPath()
	}
	r := &DeviceRegistry{path: path, Data: map[string]DeviceLocalMeta{}}
	_ = r.load()
	return r
}

func (r *DeviceRegistry) load() error {
	raw, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return json.Unmarshal(raw, &r.Data)
}

func (r *DeviceRegistry) save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, err := json.MarshalIndent(r.Data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, b, 0o600)
}

// Get returns metadata for a MAC, if any.
func (r *DeviceRegistry) Get(mac string) DeviceLocalMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Data[strings.ToUpper(strings.TrimSpace(mac))]
}

// SetDisplayName saves the local display name (empty clears override).
func (r *DeviceRegistry) SetDisplayName(mac, displayName string) error {
	mac = strings.ToUpper(strings.TrimSpace(mac))
	if mac == "" {
		return errors.New("empty device id")
	}
	r.mu.Lock()
	if r.Data == nil {
		r.Data = map[string]DeviceLocalMeta{}
	}
	dn := strings.TrimSpace(displayName)
	if dn == "" {
		delete(r.Data, mac)
	} else {
		r.Data[mac] = DeviceLocalMeta{DisplayName: dn}
	}
	r.mu.Unlock()
	return r.save()
}
