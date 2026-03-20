package wyzeferal

import "strings"

// UICategory hints the dashboard which controls to render.
const (
	UICategoryPowerSwitch = "power_switch" // binary on/off — toggle
	UICategoryDimmer      = "dimmer"       // brightness (future Wyze bulb support)
	UICategoryGeneric     = "generic"      // show status only
)

// CategorizeDeviceUI maps Wyze product_type to a dashboard control family.
func CategorizeDeviceUI(d WyzeDevice) string {
	t := strings.ToLower(strings.TrimSpace(d.Type))
	switch t {
	case "plug", "outdoorplug", "wall_switch", "wallswitch":
		return UICategoryPowerSwitch
	}
	if strings.Contains(t, "bulb") || strings.Contains(t, "light") || strings.Contains(t, "lamp") {
		return UICategoryDimmer
	}
	return UICategoryPowerSwitch
}

// IncludeDeviceOnDashboard filters out non-controllable clutter (cameras, etc.).
func IncludeDeviceOnDashboard(d WyzeDevice) bool {
	t := strings.ToLower(strings.TrimSpace(d.Type))
	blocked := []string{"camera", "lock", "sensor", "scale", "band", "watch", "thermostat", "gateway", "hub"}
	for _, b := range blocked {
		if strings.Contains(t, b) {
			return false
		}
	}
	return true
}

// EnrichedDevice is the JSON shape returned to the web UI.
type EnrichedDevice struct {
	ID               string `json:"id"` // MAC — stable primary key
	MAC              string `json:"mac"`
	ProductModel     string `json:"product_model"`
	ProductType      string `json:"product_type"`
	WyzeNickname     string `json:"wyze_nickname"`
	LocalDisplayName string `json:"local_display_name,omitempty"`
	DisplayName      string `json:"display_name"`
	IsOnline         bool   `json:"is_online"`
	SwitchState      int    `json:"switch_state"`
	DeviceParams     any    `json:"device_params,omitempty"`
	UICategory       string `json:"ui_category"`
	// DimmerPlaceholder: Wyze Open API path for brightness not wired yet — UI can show disabled slider.
	DimmerSupported bool `json:"dimmer_supported"`
}

// BuildEnrichedDevice merges Wyze payload + local registry entry.
func BuildEnrichedDevice(d WyzeDevice, local DeviceLocalMeta) EnrichedDevice {
	mac := strings.TrimSpace(d.EffectiveMAC())
	if mac != "" {
		mac = strings.ToUpper(mac)
	}
	wyzeNick := strings.TrimSpace(d.Nickname)
	localName := strings.TrimSpace(local.DisplayName)
	display := localName
	if display == "" {
		display = wyzeNick
	}
	if display == "" {
		display = mac
	}
	cat := CategorizeDeviceUI(d)
	dimmerOK := cat == UICategoryDimmer
	return EnrichedDevice{
		ID:               mac,
		MAC:              mac,
		ProductModel:     d.Model,
		ProductType:      d.Type,
		WyzeNickname:     wyzeNick,
		LocalDisplayName: localName,
		DisplayName:      display,
		IsOnline:         d.IsOnline,
		SwitchState:      d.EffectiveSwitchState(),
		DeviceParams:     d.DeviceParams,
		UICategory:       cat,
		DimmerSupported:  dimmerOK,
	}
}
