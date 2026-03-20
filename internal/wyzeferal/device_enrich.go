package wyzeferal

import "strings"

// UICategory hints the dashboard which controls to render.
const (
	UICategoryPowerSwitch = "power_switch" // binary on/off — toggle
	UICategoryDimmer      = "dimmer"       // dimmable white bulb (brightness only)
	UICategoryColorBulb   = "color_bulb"  // color + brightness capable
	UICategoryGeneric     = "generic"      // show status only
)

// colorCapableModels is the known list of Wyze models that support RGB color.
// Wyze mesh lights use "HL_" prefix; older series use "WLPA" prefix.
var colorCapableModels = []string{
	"HL_BR30C",  // Wyze Bulb Color BR30 (recessed, mesh)
	"HL_A19C",   // Wyze Bulb Color A19 (mesh, gen 1)
	"HL_A19C2",  // Wyze Bulb Color A19 (mesh, gen 2)
	"HL_WBC",    // Wyze Bulb Color (compact mesh)
	"WLPA19C",   // Wyze Bulb Color (older non-mesh A19)
	"WLPAC",     // Wyze Bulb Color (older compact)
}

// supportsColor returns true if the device is a known RGB color-capable model.
func supportsColor(d WyzeDevice) bool {
	m := strings.ToUpper(strings.TrimSpace(d.Model))
	for _, km := range colorCapableModels {
		if strings.HasPrefix(m, km) {
			return true
		}
	}
	// Type-level hint — e.g. "color_bulb" from some API responses.
	t := strings.ToLower(strings.TrimSpace(d.Type))
	return strings.Contains(t, "color")
}

// CategorizeDeviceUI maps Wyze device to a dashboard control family.
func CategorizeDeviceUI(d WyzeDevice) string {
	t := strings.ToLower(strings.TrimSpace(d.Type))
	switch t {
	case "plug", "outdoorplug", "wall_switch", "wallswitch":
		return UICategoryPowerSwitch
	}
	if strings.Contains(t, "bulb") || strings.Contains(t, "light") || strings.Contains(t, "lamp") {
		if supportsColor(d) {
			return UICategoryColorBulb
		}
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
	DimmerSupported  bool   `json:"dimmer_supported"`
	ColorSupported   bool   `json:"color_supported"`
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
		DimmerSupported:  cat == UICategoryDimmer || cat == UICategoryColorBulb,
		ColorSupported:   cat == UICategoryColorBulb,
	}
}
