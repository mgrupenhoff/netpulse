// fdbsticky694_test.go — Issue #694: puertas transitorias de la memo sticky.
//
// Un cliente cableado y callado de un dumb AP (mismo L2 que el gateway) alterna
// entre colgarse del AP y del gateway cuando (a) la brMac del AP no está en el
// FDB del gateway ese tick y (b) el sondeo del AP falla. Estos tests cubren la
// clasificación PERSISTENTE de uplink, la no-sobrescritura por parte del
// gateway, el refresco de la memo para dispositivos online y el determinismo.
package adapters

import (
	"testing"
	"time"

	"github.com/gnacho/netpulse/agent/macaddr"
)

// TestUpdateFdbMemoGatewayUplinkWithoutApBrMac (#694): cuando la brMac del AP
// no está en el FDB del gateway este tick, el puerto gateway→AP sigue
// clasificado como uplink por la memoria persistente, de modo que updateFdbMemo
// deja la memo en el AP y buildDevices atribuye al AP (no al gateway).
func TestUpdateFdbMemoGatewayUplinkWithoutApBrMac(t *testing.T) {
	l := NewLive(nil, nil, []RouterConfig{{ID: "gateway", Name: "GW", IsGateway: true}}, nil)
	now := int64(1_700_000_000_000)
	gwMAC := macaddr.MustParse("60:60:60:60:60:60")
	apMAC := macaddr.MustParse("AA:AA:AA:AA:AA:AA")
	cliMAC := macaddr.MustParse("44:44:44:44:44:44")
	// Tick 1: el gateway aprende la brMac del AP en lan4 → clasifica lan4 como uplink.
	polled1 := map[string]*routerPolled{
		"gateway": {cfg: RouterConfig{ID: "gateway", IsGateway: true}, brMac: gwMAC,
			fdb: map[macaddr.MAC]string{gwMAC: "lan1", apMAC: "lan4"}},
		"ap": {cfg: RouterConfig{ID: "ap", Name: "Luizjana2"}, brMac: apMAC,
			fdb: map[macaddr.MAC]string{apMAC: "lan1", cliMAC: "lan3"}},
	}
	l.updateFdbMemo(polled1, now)

	// Tick 2: la brMac del AP ya NO está en el FDB del gateway, pero el cliente
	// sí aparece en lan4 (uplink). La clasificación persistente debe impedir que
	// el gateway grabe lan4 como boca local y pise la observación del AP.
	polled2 := map[string]*routerPolled{
		"gateway": {cfg: RouterConfig{ID: "gateway", IsGateway: true}, brMac: gwMAC,
			fdb: map[macaddr.MAC]string{gwMAC: "lan1", cliMAC: "lan4"}},
		"ap": {cfg: RouterConfig{ID: "ap", Name: "Luizjana2"}, brMac: apMAC,
			fdb: map[macaddr.MAC]string{apMAC: "lan1", cliMAC: "lan3"}},
	}
	memo := l.updateFdbMemo(polled2, now)
	if m, ok := memo[cliMAC]; !ok || m.routerID != "ap" || m.port != "lan3" {
		t.Fatalf("la memo debe quedar en el AP (lan3), got %+v", memo[cliMAC])
	}
	devs := l.buildDevices(polled2, "")
	if d := mustDevice(t, devs, "44:44:44:44:44:44"); d.RouterID != "ap" {
		t.Fatalf("buildDevices debería atribuir al AP, got %q", d.RouterID)
	}
}

// TestBuildDevicesKeepsApWhenApPollFails (#694): con el sondeo del AP fallido
// (el AP no está en polled), una memo previa del AP conserva la atribución.
func TestBuildDevicesKeepsApWhenApPollFails(t *testing.T) {
	l := NewLive(nil, nil, []RouterConfig{{ID: "gateway", Name: "GW", IsGateway: true}}, nil)
	now := time.Now().UnixMilli()
	gwMAC := macaddr.MustParse("60:60:60:60:60:60")
	cliMAC := macaddr.MustParse("44:44:44:44:44:44")
	l.fdbMemo[cliMAC] = fdbPortMemo{routerID: "ap", port: "lan3", ts: now}
	polled := map[string]*routerPolled{
		"gateway": {cfg: RouterConfig{ID: "gateway", IsGateway: true}, brMac: gwMAC,
			fdb: map[macaddr.MAC]string{gwMAC: "lan1"},
			arp: map[macaddr.MAC]string{cliMAC: "192.168.13.123"}},
	}
	devs := l.buildDevices(polled, "")
	if d := mustDevice(t, devs, "44:44:44:44:44:44"); d.RouterID != "ap" {
		t.Fatalf("RouterID=%q (want ap por la memo, aunque el AP falló el sondeo)", d.RouterID)
	}
}

// TestUpdateFdbMemoRefreshesOnlineDevice (#694): una memo caducada se conserva
// (refrescando su ts) mientras el dispositivo siga online por ARP, aunque ya no
// hable por FDB.
func TestUpdateFdbMemoRefreshesOnlineDevice(t *testing.T) {
	l := NewLive(nil, nil, []RouterConfig{{ID: "gateway", Name: "GW", IsGateway: true}}, nil)
	now := int64(1_700_000_000_000)
	gwMAC := macaddr.MustParse("60:60:60:60:60:60")
	cliMAC := macaddr.MustParse("44:44:44:44:44:44")
	l.fdbMemo[cliMAC] = fdbPortMemo{
		routerID: "ap", port: "lan3",
		ts: now - fdbStickyTTL.Milliseconds() - 5_000,
	}
	polled := map[string]*routerPolled{
		"gateway": {cfg: RouterConfig{ID: "gateway", IsGateway: true}, brMac: gwMAC,
			fdb: map[macaddr.MAC]string{gwMAC: "lan1"},
			arp: map[macaddr.MAC]string{cliMAC: "192.168.13.123"}},
	}
	memo := l.updateFdbMemo(polled, now)
	m, ok := memo[cliMAC]
	if !ok {
		t.Fatalf("la memo del device online no debería podarse: %+v", memo)
	}
	if m.routerID != "ap" || m.port != "lan3" {
		t.Fatalf("memo=%+v (want ap/lan3)", m)
	}
	if now-m.ts > fdbStickyTTL.Milliseconds() {
		t.Fatalf("la memo online debería refrescar su ts, got ts=%d", m.ts)
	}
}

// TestUpdateFdbMemoDeterministicWinner (#694): con el gateway y el AP viendo la
// misma MAC, el ganador de la memo no debe variar entre llamadas (antes dependía
// del orden aleatorio de iteración del mapa).
func TestUpdateFdbMemoDeterministicWinner(t *testing.T) {
	l := NewLive(nil, nil, []RouterConfig{{ID: "gateway", Name: "GW", IsGateway: true}}, nil)
	now := int64(1_700_000_000_000)
	gwMAC := macaddr.MustParse("60:60:60:60:60:60")
	apMAC := macaddr.MustParse("AA:AA:AA:AA:AA:AA")
	cliMAC := macaddr.MustParse("44:44:44:44:44:44")
	// Clasificación persistente: lan4 del gateway es el uplink hacia el AP.
	l.uplinkPorts["gateway"] = map[string]bool{"lan4": true}
	polled := map[string]*routerPolled{
		"gateway": {cfg: RouterConfig{ID: "gateway", IsGateway: true}, brMac: gwMAC,
			fdb: map[macaddr.MAC]string{gwMAC: "lan1", cliMAC: "lan4"}},
		"ap": {cfg: RouterConfig{ID: "ap"}, brMac: apMAC,
			fdb: map[macaddr.MAC]string{apMAC: "lan1", cliMAC: "lan3"}},
	}
	for i := 0; i < 50; i++ {
		memo := l.updateFdbMemo(polled, now)
		if m, ok := memo[cliMAC]; !ok || m.routerID != "ap" || m.port != "lan3" {
			t.Fatalf("iteración %d: el ganador debería ser siempre ap/lan3, got %+v", i, memo[cliMAC])
		}
	}
}
