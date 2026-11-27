package adapters

import (
	"encoding/json"
	"testing"

	"github.com/gnacho/netpulse/agent/macaddr"
)

// TestMACFieldsJSONOmitZero comprueba que los campos opcionales de tipo
// macaddr.MAC (Router.MAC, DistributionNode.Mac, Radio.BSSID, EthPort.DeviceMac)
// omiten su clave JSON cuando son el valor cero (omitzero) y la emiten en
// forma canónica en mayúsculas cuando son válidos.
func TestMACFieldsJSONOmitZero(t *testing.T) {
	t.Run("router sin MAC omite clave", func(t *testing.T) {
		raw, err := json.Marshal(Router{ID: "ext1", Name: "ONT"})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if _, ok := m["mac"]; ok {
			t.Fatalf("Router sin MAC no debe emitir clave \"mac\": %s", raw)
		}
	})

	t.Run("router con MAC emite forma canónica", func(t *testing.T) {
		raw, err := json.Marshal(Router{ID: "gw", MAC: macaddr.MustParse("aa-bb-cc-dd-ee-ff")})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if got, _ := m["mac"].(string); got != "AA:BB:CC:DD:EE:FF" {
			t.Fatalf("mac = %q, esperaba AA:BB:CC:DD:EE:FF", got)
		}
	})

	t.Run("distnode sin MAC omite clave", func(t *testing.T) {
		raw, err := json.Marshal(DistributionNode{ID: "dist-gw-lan1", Kind: "inferred"})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if _, ok := m["mac"]; ok {
			t.Fatalf("DistributionNode sin MAC no debe emitir clave \"mac\": %s", raw)
		}
	})

	t.Run("distnode con MAC hace round-trip", func(t *testing.T) {
		in := DistributionNode{ID: "dist-living-lan3", Kind: "managed", Mac: macaddr.MustParse("28:c6:8e:1d:90:44")}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		var out DistributionNode
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if out.Mac != in.Mac {
			t.Fatalf("round-trip Mac = %s, esperaba %s", out.Mac, in.Mac)
		}
	})

	t.Run("radio sin BSSID omite clave", func(t *testing.T) {
		raw, err := json.Marshal(Radio{Name: "5 GHz", Channel: 36})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if _, ok := m["bssid"]; ok {
			t.Fatalf("Radio sin BSSID no debe emitir clave \"bssid\": %s", raw)
		}
	})

	t.Run("radio con BSSID hace round-trip", func(t *testing.T) {
		in := Radio{Name: "5 GHz", Channel: 36, BSSID: macaddr.MustParse("9c:9d:7e:1b:ea:b3")}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		var out Radio
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if out.BSSID != in.BSSID {
			t.Fatalf("round-trip BSSID = %s, esperaba %s", out.BSSID, in.BSSID)
		}
	})

	t.Run("ethport sin DeviceMac omite clave", func(t *testing.T) {
		raw, err := json.Marshal(EthPort{ID: "lan1", Label: "LAN 1", Up: true})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if _, ok := m["deviceMac"]; ok {
			t.Fatalf("EthPort sin DeviceMac no debe emitir clave \"deviceMac\": %s", raw)
		}
	})

	t.Run("ethport con DeviceMac hace round-trip", func(t *testing.T) {
		in := EthPort{ID: "lan1", Label: "LAN 1", Up: true, DeviceMac: macaddr.MustParse("78:55:36:02:58:ba")}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		var out EthPort
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if out.DeviceMac != in.DeviceMac {
			t.Fatalf("round-trip DeviceMac = %s, esperaba %s", out.DeviceMac, in.DeviceMac)
		}
	})

	t.Run("device sin MAC serializa cadena vacía (no omitzero)", func(t *testing.T) {
		raw, err := json.Marshal(Device{ID: "dev1", Name: "dev"})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		v, ok := m["mac"]
		if !ok {
			t.Fatalf("Device.MAC no debe omitirse cuando es cero: %s", raw)
		}
		if s, isStr := v.(string); !isStr || s != "" {
			t.Fatalf("Device.MAC cero = %#v, esperaba \"\"", v)
		}
	})

	t.Run("device AliasMacs omite nil y serializa MACs canónicas", func(t *testing.T) {
		rawEmpty, err := json.Marshal(Device{ID: "dev1"})
		if err != nil {
			t.Fatal(err)
		}
		var mEmpty map[string]any
		if err := json.Unmarshal(rawEmpty, &mEmpty); err != nil {
			t.Fatal(err)
		}
		if _, ok := mEmpty["aliasMacs"]; ok {
			t.Fatalf("AliasMacs nil debe omitirse: %s", rawEmpty)
		}

		rawPop, err := json.Marshal(Device{
			ID:        "dev1",
			AliasMacs: []macaddr.MAC{macaddr.MustParse("aa-bb-cc-dd-ee-01"), macaddr.MustParse("11:22:33:44:55:66")},
		})
		if err != nil {
			t.Fatal(err)
		}
		var mPop map[string]any
		if err := json.Unmarshal(rawPop, &mPop); err != nil {
			t.Fatal(err)
		}
		got, ok := mPop["aliasMacs"].([]any)
		if !ok || len(got) != 2 || got[0] != "AA:BB:CC:DD:EE:01" || got[1] != "11:22:33:44:55:66" {
			t.Fatalf("aliasMacs = %#v, esperaba [AA:BB:CC:DD:EE:01 11:22:33:44:55:66]", mPop["aliasMacs"])
		}
	})

	t.Run("networkinfo ClientMacs omite nil y serializa MACs canónicas", func(t *testing.T) {
		rawEmpty, err := json.Marshal(NetworkInfo{Name: "lan"})
		if err != nil {
			t.Fatal(err)
		}
		var mEmpty map[string]any
		if err := json.Unmarshal(rawEmpty, &mEmpty); err != nil {
			t.Fatal(err)
		}
		if _, ok := mEmpty["clientMacs"]; ok {
			t.Fatalf("ClientMacs nil debe omitirse: %s", rawEmpty)
		}

		rawPop, err := json.Marshal(NetworkInfo{
			Name:       "lan",
			ClientMacs: []macaddr.MAC{macaddr.MustParse("aa:bb:cc:11:22:33")},
		})
		if err != nil {
			t.Fatal(err)
		}
		var mPop map[string]any
		if err := json.Unmarshal(rawPop, &mPop); err != nil {
			t.Fatal(err)
		}
		got, ok := mPop["clientMacs"].([]any)
		if !ok || len(got) != 1 || got[0] != "AA:BB:CC:11:22:33" {
			t.Fatalf("clientMacs = %#v, esperaba [AA:BB:CC:11:22:33]", mPop["clientMacs"])
		}
	})
}
