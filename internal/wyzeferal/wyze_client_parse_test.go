package wyzeferal

import (
	"encoding/json"
	"testing"
)

func TestParseDeviceListFromData_shapes(t *testing.T) {
	t.Parallel()
	rawObj := []byte(`{"total":1,"device_list":[{"mac":"AA","product_model":"WLPP1","nickname":"P1","product_type":"Plug","is_online":true}]}`)
	devs, err := parseDeviceListFromData(rawObj)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].EffectiveMAC() != "AA" {
		t.Fatalf("got %+v", devs)
	}

	rawArr := []byte(`[{"device_mac":"BB","product_model":"X","nickname":"N","product_type":"Plug"}]`)
	devs, err = parseDeviceListFromData(rawArr)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].EffectiveMAC() != "BB" {
		t.Fatalf("got %+v", devs)
	}

	empty := []byte(`[]`)
	devs, err = parseDeviceListFromData(empty)
	if err != nil || len(devs) != 0 {
		t.Fatalf("empty: err=%v len=%d", err, len(devs))
	}
}

func TestWyzeDeviceJSON_roundTrip(t *testing.T) {
	t.Parallel()
	in := `{"device_mac":"ZZ:11","product_model":"M","nickname":"Hi","product_type":"Plug"}`
	var d WyzeDevice
	if err := json.Unmarshal([]byte(in), &d); err != nil {
		t.Fatal(err)
	}
	if d.EffectiveMAC() != "ZZ:11" {
		t.Fatal(d.EffectiveMAC())
	}
}
