package adapters

import (
	"slices"
	"testing"

	"github.com/gnacho/netpulse/agent/macaddr"
	"github.com/gnacho/netpulse/agent/probe"
)

// Un fallo transitorio de ubus en el sondeo SSH no debe olvidar el uplink:
// sin él, Automático volvería a contar como clientes a los vecinos de la red
// de arriba durante el minuto que se cachea el fallo.
func TestSSHUplinkDevsKeepsLastGoodOnUbusFailure(t *testing.T) {
	cached := []string{"phy1-sta0", "phy1-sta0"}

	got := sshUplinkDevs(probe.WanInfo{Device: "phy1-sta0", Port: "phy1-sta0"}, nil)
	if !slices.Equal(got, cached) {
		t.Fatalf("lectura buena: got %v, want %v", got, cached)
	}
	if got := sshUplinkDevs(probe.WanInfo{}, cached); !slices.Equal(got, cached) {
		t.Fatalf("con ubus fallido debía conservar la última lista buena: got %v", got)
	}

	p := &routerPolled{
		cfg:        RouterConfig{ID: "travel", Name: "travel"},
		uplinkDevs: sshUplinkDevs(probe.WanInfo{}, cached),
		arp: map[macaddr.MAC]string{
			macaddr.MustParse("AA:BB:CC:00:00:01"): "10.20.0.1",
			macaddr.MustParse("AA:BB:CC:00:00:02"): "192.168.2.50",
		},
		arpDevs: map[macaddr.MAC]string{
			macaddr.MustParse("AA:BB:CC:00:00:01"): "phy1-sta0",
			macaddr.MustParse("AA:BB:CC:00:00:02"): "br-lan",
		},
	}
	l := NewLive(nil, nil, nil, nil)
	devs := l.buildDevices(map[string]*routerPolled{"travel": p}, "")
	if len(devs) != 1 || devs[0].MAC != macaddr.MustParse("AA:BB:CC:00:00:02") {
		t.Fatalf("con ubus fallido solo debía contar el vecino de la LAN, got %+v", devs)
	}
}
