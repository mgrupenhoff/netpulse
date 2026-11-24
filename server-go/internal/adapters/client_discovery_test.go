// client_discovery_test.go — Regla de descubrimiento de clientes por red
// lógica de OpenWrt (y respaldo por dispositivo Linux) en el servidor: modo
// automático, overrides explícitos ("on" / "off"), resolución cuando varias
// redes comparten dispositivo, interacción con WanSide (#1485) en el gateway y
// construcción de RouterDetail.Discovery.
package adapters

import (
	"context"
	"reflect"
	"testing"

	"github.com/gnacho/netpulse/agent/probe"
)

func boolPtr(b bool) *bool { return &b }

func travelNetIfaces() []probe.NetIface {
	return []probe.NetIface{
		{Name: "lan", Proto: "static", Up: true, Devices: []string{"br-lan"}},
		{Name: "wan", Proto: "dhcp", Up: false, Devices: []string{"eth0.2"}},
		{Name: "wan6", Proto: "dhcpv6", Up: false, Devices: []string{"eth0.2"}},
		{Name: "wg0", Proto: "wireguard", Up: true, Devices: []string{"wg0"}},
		{Name: "wwan", Proto: "dhcp", Up: true, Devices: []string{"phy1-sta0"}},
	}
}

func travelPolled(ov DiscoveryOverrides) *routerPolled {
	return &routerPolled{
		cfg: RouterConfig{
			ID:                 "travel",
			Name:               "travel",
			Host:               "192.168.2.1",
			DiscoveryOverrides: ov,
		},
		wanInfo:    probe.WanInfo{Device: "phy1-sta0", Port: "phy1-sta0"},
		uplinkDevs: []string{"phy1-sta0"},
		netIfaces:  travelNetIfaces(),
		arp: map[string]string{
			"AA:BB:CC:00:00:01": "10.20.0.177",  // vecino en wwan (uplink)
			"AA:BB:CC:00:00:02": "192.168.2.50", // vecino en lan
		},
		arpDevs: map[string]string{
			"AA:BB:CC:00:00:01": "phy1-sta0",
			"AA:BB:CC:00:00:02": "br-lan",
		},
	}
}

// En un router de viaje, buildDiscoveryIfaces emite una fila por cada red en
// orden del dump (sin filas device cuando todos los dispositivos pertenecen a
// una red), con Auto=false solo en wwan y el conteo de vecinos confirmados
// correcto.
func TestClientDiscoveryTravelRouterNetworkRows(t *testing.T) {
	p := travelPolled(DiscoveryOverrides{})
	p.arp["AA:BB:CC:00:00:03"] = "10.20.0.178"
	p.arpDevs["AA:BB:CC:00:00:03"] = "phy1-sta0"
	p.arpStale = map[string]bool{"AA:BB:CC:00:00:03": true} // stale no suma

	got := buildDiscoveryIfaces(p)
	want := []DiscoveryIface{
		{Kind: "network", Name: "lan", Devices: []string{"br-lan"}, Up: boolPtr(true), Mode: "auto", Auto: true, Effective: true, Neighbours: 1},
		{Kind: "network", Name: "wan", Devices: []string{"eth0.2"}, Up: boolPtr(false), Mode: "auto", Auto: true, Effective: true, Neighbours: 0},
		{Kind: "network", Name: "wan6", Devices: []string{"eth0.2"}, Up: boolPtr(false), Mode: "auto", Auto: true, Effective: true, Neighbours: 0},
		{Kind: "network", Name: "wg0", Devices: []string{"wg0"}, Up: boolPtr(true), Mode: "auto", Auto: true, Effective: true, Neighbours: 0},
		{Kind: "network", Name: "wwan", Devices: []string{"phy1-sta0"}, Up: boolPtr(true), Mode: "auto", Auto: false, Effective: false, Neighbours: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filas de descubrimiento del router de viaje:\ngot  %+v\nwant %+v", got, want)
	}

	// En automático solo cuenta el vecino de br-lan, no el de phy1-sta0.
	l := NewLive(nil, nil, nil, nil)
	devs := l.buildDevices(map[string]*routerPolled{"travel": p}, "")
	if len(devs) != 1 || devs[0].MAC != "AA:BB:CC:00:00:02" {
		t.Fatalf("en automático solo debe contar el vecino de lan: %+v", devs)
	}
}

// Un vecino en un dispositivo que no pertenece a ninguna red (p. ej. eth3)
// aparece como fila device y cuenta como cliente en automático.
func TestClientDiscoveryUnassignedDeviceRow(t *testing.T) {
	p := travelPolled(DiscoveryOverrides{})
	p.arp["AA:BB:CC:00:00:09"] = "192.168.9.10"
	p.arpDevs["AA:BB:CC:00:00:09"] = "eth3"

	rows := buildDiscoveryIfaces(p)
	if len(rows) != 6 {
		t.Fatalf("esperaba 5 redes + 1 fila device, obtuve %d: %+v", len(rows), rows)
	}
	wantDev := DiscoveryIface{Kind: "device", Name: "eth3", Mode: "auto", Auto: true, Effective: true, Neighbours: 1}
	if !reflect.DeepEqual(rows[5], wantDev) {
		t.Fatalf("fila device de respaldo:\ngot  %+v\nwant %+v", rows[5], wantDev)
	}

	l := NewLive(nil, nil, nil, nil)
	devs := l.buildDevices(map[string]*routerPolled{"travel": p}, "")
	gotMACs := map[string]bool{}
	for _, d := range devs {
		gotMACs[d.MAC] = true
	}
	if !gotMACs["AA:BB:CC:00:00:02"] || !gotMACs["AA:BB:CC:00:00:09"] || gotMACs["AA:BB:CC:00:00:01"] {
		t.Fatalf("esperaba clientes en br-lan y eth3 (sin phy1-sta0): %+v", devs)
	}
}

// Con Networks["wwan"]="on", los vecinos del uplink pasan a contar como clientes.
func TestClientDiscoveryNetworkOverrideOnWwan(t *testing.T) {
	p := travelPolled(DiscoveryOverrides{
		Networks: map[string]DiscoveryMode{"wwan": "on"},
	})
	l := NewLive(nil, nil, nil, nil)
	devs := l.buildDevices(map[string]*routerPolled{"travel": p}, "")
	if len(devs) != 2 {
		t.Fatalf("con wwan='on' deben contar ambos vecinos (lan y wwan): %+v", devs)
	}
}

// Con Networks["lan"]="off", los vecinos de br-lan se descartan.
func TestClientDiscoveryNetworkOverrideOffLan(t *testing.T) {
	p := travelPolled(DiscoveryOverrides{
		Networks: map[string]DiscoveryMode{"lan": "off"},
	})
	l := NewLive(nil, nil, nil, nil)
	if devs := l.buildDevices(map[string]*routerPolled{"travel": p}, ""); len(devs) != 0 {
		t.Fatalf("con lan='off' y wwan en automático no debe haber clientes: %+v", devs)
	}
}

// Dos redes que comparten dispositivo (wan y wan6 sobre eth0.2) cuando eth0.2
// es el uplink: ambas tienen Auto=false; explícito vence a automático; entre
// explícitos, "off" vence a "on".
func TestClientDiscoverySharedDeviceResolution(t *testing.T) {
	makePolled := func(nets map[string]DiscoveryMode) *routerPolled {
		return &routerPolled{
			cfg: RouterConfig{
				ID:                 "rt",
				Name:               "rt",
				Host:               "192.168.1.1",
				DiscoveryOverrides: DiscoveryOverrides{Networks: nets},
			},
			wanInfo:   probe.WanInfo{Device: "eth0.2"},
			netIfaces: travelNetIfaces(),
			arp:       map[string]string{"AA:BB:CC:00:00:55": "198.51.100.1"},
			arpDevs:   map[string]string{"AA:BB:CC:00:00:55": "eth0.2"},
		}
	}

	t.Run("automatico desactiva wan y wan6", func(t *testing.T) {
		p := makePolled(nil)
		rows := buildDiscoveryIfaces(p)
		for _, r := range rows {
			if r.Name == "wan" || r.Name == "wan6" {
				if r.Auto || r.Effective || r.Neighbours != 1 {
					t.Fatalf("fila %s esperaba Auto=false Effective=false Neighbours=1: %+v", r.Name, r)
				}
			}
		}
		if resolveDiscovery(p, "eth0.2", nil).allowed() {
			t.Fatal("eth0.2 debía quedar bloqueado en automático")
		}
	})

	t.Run("wan on con wan6 en automatico permite el dispositivo", func(t *testing.T) {
		p := makePolled(map[string]DiscoveryMode{"wan": "on"})
		if !resolveDiscovery(p, "eth0.2", nil).allowed() {
			t.Fatal("wan='on' + wan6='auto' debía permitir eth0.2 (explícito vence a automático)")
		}
		if !discoveryExplicitOn(p, "eth0.2") {
			t.Fatal("wan='on' + wan6='auto' debía considerarse explicitOn")
		}
	})

	t.Run("wan on con wan6 off bloquea el dispositivo", func(t *testing.T) {
		p := makePolled(map[string]DiscoveryMode{"wan": "on", "wan6": "off"})
		if resolveDiscovery(p, "eth0.2", nil).allowed() {
			t.Fatal("wan='on' + wan6='off' debía bloquear eth0.2 ('off' vence a 'on')")
		}
		if discoveryExplicitOn(p, "eth0.2") {
			t.Fatal("wan='on' + wan6='off' no debe ser explicitOn")
		}
	})

	// El orden de las redes no importa: un Off en la primera corta el bucle y
	// el On de la segunda no lo deshace.
	t.Run("wan off con wan6 on bloquea el dispositivo", func(t *testing.T) {
		p := makePolled(map[string]DiscoveryMode{"wan": "off", "wan6": "on"})
		if resolveDiscovery(p, "eth0.2", nil).allowed() {
			t.Fatal("wan='off' + wan6='on' debía bloquear eth0.2 ('off' vence a 'on')")
		}
		if discoveryExplicitOn(p, "eth0.2") {
			t.Fatal("wan='off' + wan6='on' no debe ser explicitOn")
		}
	})
}

// En el gateway, el vecino de wanPort es WanSide por defecto (#1485) y solo
// deja de marcarse WanSide cuando hay un override explícito "on" por red.
func TestClientDiscoveryGatewayWanPortNetworkOverride(t *testing.T) {
	gwCfg := RouterConfig{ID: "gw", Name: "gw", Host: "192.168.1.1", IsGateway: true}
	l := NewLive(nil, nil, []RouterConfig{gwCfg}, nil)
	ifaces := []probe.NetIface{
		{Name: "lan", Proto: "static", Up: true, Devices: []string{"br-lan"}},
		{Name: "wan", Proto: "dhcp", Up: true, Devices: []string{"eth0.2"}},
		{Name: "wan6", Proto: "dhcpv6", Up: true, Devices: []string{"eth0.2"}},
	}

	makeGW := func(cfg RouterConfig) map[string]*routerPolled {
		return map[string]*routerPolled{
			"gw": {
				cfg:       cfg,
				wanInfo:   probe.WanInfo{Device: "eth0.2", Port: "eth0.2"},
				netIfaces: ifaces,
				arp:       map[string]string{"AA:BB:CC:00:00:99": "192.168.100.1"},
				arpDevs:   map[string]string{"AA:BB:CC:00:00:99": "eth0.2"},
			},
		}
	}

	// Sin override: sigue siendo WanSide.
	devs := l.buildDevices(makeGW(gwCfg), "eth0.2")
	if len(devs) != 1 || !devs[0].WanSide {
		t.Fatalf("sin override el vecino de wanPort debe seguir siendo WanSide: %+v", devs)
	}

	// Con Networks["wan"]="on": deja de ser WanSide y entra como cliente normal.
	gwOn := gwCfg
	gwOn.DiscoveryOverrides = DiscoveryOverrides{Networks: map[string]DiscoveryMode{"wan": "on"}}
	l.SetRouters([]RouterConfig{gwOn})
	devs = l.buildDevices(makeGW(gwOn), "eth0.2")
	if len(devs) != 1 || devs[0].WanSide {
		t.Fatalf("con Networks['wan']='on' el vecino debe ser cliente y no WanSide: %+v", devs)
	}

	// Con wan="on" y wan6="off": off vence a on, así que no es explicitOn -> WanSide.
	gwConflict := gwCfg
	gwConflict.DiscoveryOverrides = DiscoveryOverrides{Networks: map[string]DiscoveryMode{"wan": "on", "wan6": "off"}}
	l.SetRouters([]RouterConfig{gwConflict})
	devs = l.buildDevices(makeGW(gwConflict), "eth0.2")
	if len(devs) != 1 || !devs[0].WanSide {
		t.Fatalf("con wan='on' y wan6='off' debe seguir siendo WanSide: %+v", devs)
	}
}

// En el gateway con IP de puerta de enlace WAN (#2183/#1791): sin override no
// entra ni el peer de la ruta por defecto (#1791) ni el vecino del segmento
// (wanIgnored, #2188). Con Networks["wan"]="on", el vecino confirmado pasa a
// ser un cliente normal (WanSide=false), pero el peer sigue absorbido por #1791.
func TestClientDiscoveryGatewayWanPortWithGatewayIP(t *testing.T) {
	const (
		peer      = "0C:EF:15:00:00:01"
		neighbour = "0C:EF:15:00:00:02"
	)
	gwCfg := RouterConfig{ID: "gw", Name: "gw", Host: "192.168.1.1", IsGateway: true}
	l := NewLive(nil, nil, []RouterConfig{gwCfg}, nil)
	ifaces := []probe.NetIface{
		{Name: "lan", Proto: "static", Up: true, Devices: []string{"br-lan"}},
		{Name: "wan", Proto: "dhcp", Up: true, Devices: []string{"eth0.2"}},
		{Name: "wan6", Proto: "dhcpv6", Up: true, Devices: []string{"eth0.2"}},
	}

	makeGW := func(cfg RouterConfig) map[string]*routerPolled {
		return map[string]*routerPolled{
			"gw": {
				cfg:       cfg,
				wanInfo:   probe.WanInfo{Device: "eth0.2", Port: "eth0.2", Gateway: "203.0.113.1"},
				netIfaces: ifaces,
				arp: map[string]string{
					peer:      "203.0.113.1",
					neighbour: "203.0.113.248",
				},
				arpDevs: map[string]string{
					peer:      "eth0.2",
					neighbour: "eth0.2",
				},
			},
		}
	}

	// Sin override: el peer cae en #1791 y el vecino en wanIgnored (#2188).
	if devs := l.buildDevices(makeGW(gwCfg), "eth0.2"); len(devs) != 0 {
		t.Fatalf("sin override no debe salir ni el peer ni el vecino del segmento WAN: %+v", devs)
	}

	// Con Networks["wan"]="on": el vecino confirmado entra como cliente normal,
	// pero el peer de la ruta por defecto sigue descartado por #1791.
	gwOn := gwCfg
	gwOn.DiscoveryOverrides = DiscoveryOverrides{Networks: map[string]DiscoveryMode{"wan": "on"}}
	l.SetRouters([]RouterConfig{gwOn})
	devs := l.buildDevices(makeGW(gwOn), "eth0.2")
	if len(devs) != 1 || devs[0].MAC != neighbour || devs[0].WanSide {
		t.Fatalf("con Networks['wan']='on' solo debe entrar el vecino (%s) como cliente normal (WanSide=false): %+v", neighbour, devs)
	}
}

// Una MAC solo-IPv6 cuya interfaz (reportada en arpDevs tras el recheck NDP) es
// el uplink de la unidad no se cuenta como cliente en modo automático.
func TestClientDiscoveryIPv6OnlyOnWanSkippedInAuto(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"travel": {
			cfg:        RouterConfig{ID: "travel", Name: "travel", Host: "192.168.2.1"},
			uplinkDevs: []string{"phy1-sta0"},
			arp:        map[string]string{"BB:BB:BB:00:00:77": "fe80::1"},
			arpDevs:    map[string]string{"BB:BB:BB:00:00:77": "phy1-sta0"},
		},
	}
	if devs := l.buildDevices(polled, ""); len(devs) != 0 {
		t.Fatalf("un vecino solo-IPv6 en el uplink no debe ser cliente en automático: %+v", devs)
	}
}

// Sin netIfaces (agente antiguo o equipo no-OpenWrt), RouterDetail.Discovery
// produce exactamente las filas device de respaldo, ordenadas por nombre.
func TestRouterDetailDiscoveryRowsOldAgent(t *testing.T) {
	cfg := RouterConfig{
		ID:   "travel",
		Name: "travel",
		Host: "192.168.2.1",
		DiscoveryOverrides: DiscoveryOverrides{
			Devices: map[string]DiscoveryMode{
				"eth2":      "off",
				"phy1-sta0": "on",
			},
		},
	}
	l := liveTestLive()
	l.SetRouters([]RouterConfig{
		{ID: "gw", Name: "gw", Host: "192.168.1.1", IsGateway: true},
		cfg,
	})

	// Unidad aún no sondeada -> Discovery es nil.
	detBefore, err := l.GetRouterDetail(context.Background(), "travel")
	if err != nil || detBefore == nil {
		t.Fatalf("GetRouterDetail sin sondeo: det=%+v err=%v", detBefore, err)
	}
	if detBefore.Discovery != nil {
		t.Fatalf("Discovery sin sondeo debe ser nil: %+v", detBefore.Discovery)
	}

	l.mu.Lock()
	l.lastPolled["travel"] = &routerPolled{
		cfg:        cfg,
		uplinkDevs: []string{"phy1-sta0", "wan0"},
		arp: map[string]string{
			"AA:BB:CC:00:00:01": "192.168.2.10", // confirmada en br-lan
			"AA:BB:CC:00:00:02": "192.168.2.11", // confirmada en br-lan
			"AA:BB:CC:00:00:03": "192.168.2.12", // stale en br-lan (no cuenta)
			"AA:BB:CC:00:00:04": "10.20.0.1",    // confirmada en phy1-sta0
		},
		arpStale: map[string]bool{
			"AA:BB:CC:00:00:03": true,
		},
		arpDevs: map[string]string{
			"AA:BB:CC:00:00:01": "br-lan",
			"AA:BB:CC:00:00:02": "br-lan",
			"AA:BB:CC:00:00:03": "br-lan",
			"AA:BB:CC:00:00:04": "phy1-sta0",
		},
	}
	l.mu.Unlock()

	det, err := l.GetRouterDetail(context.Background(), "travel")
	if err != nil || det == nil {
		t.Fatalf("GetRouterDetail: det=%+v err=%v", det, err)
	}
	want := []DiscoveryIface{
		{Kind: "device", Name: "br-lan", Mode: "auto", Auto: true, Effective: true, Neighbours: 2},
		{Kind: "device", Name: "eth2", Mode: "off", Auto: true, Effective: false, Neighbours: 0},
		{Kind: "device", Name: "phy1-sta0", Mode: "on", Auto: false, Effective: true, Neighbours: 1},
		{Kind: "device", Name: "wan0", Mode: "auto", Auto: false, Effective: false, Neighbours: 0},
	}
	if !reflect.DeepEqual(det.Discovery, want) {
		t.Fatalf("Discovery:\ngot  %+v\nwant %+v", det.Discovery, want)
	}
}

// Un override guardado de una red que ahora no se reporta (un tether USB
// desenchufado) sigue teniendo fila: si no, guardar otra fila desde el panel
// lo borraría sin avisar.
func TestBuildDiscoveryIfacesKeepsSavedMissingNetworkOverride(t *testing.T) {
	p := travelPolled(DiscoveryOverrides{
		Networks: map[string]DiscoveryMode{
			"wwan": DiscoveryOn,
			"usb0": DiscoveryOff, // red guardada, temporalmente ausente de netIfaces
		},
	})
	rows := buildDiscoveryIfaces(p)
	found := false
	for _, r := range rows {
		if r.Kind == DiscoveryKindNetwork && r.Name == "usb0" {
			found = true
			if r.Mode != DiscoveryOff || r.Effective {
				t.Fatalf("fila usb0 esperada mode=off effective=false, got %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("buildDiscoveryIfaces omitió el override guardado de la red ausente usb0: %+v", rows)
	}
}

// Las interfaces del uplink valen aunque la unidad no sea el gateway y no
// traiga wanInfo (sondeo SSH de una unidad que no es gateway).
func TestBuildDevicesSkipsArpOnUplinkDevs(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"travel": {
			cfg:        RouterConfig{ID: "travel", Name: "travel", Host: "192.168.2.1"},
			uplinkDevs: []string{"phy1-sta0", "phy1-sta0"},
			arp: map[string]string{
				"AA:BB:CC:00:00:01": "10.20.0.177",
				"AA:BB:CC:00:00:02": "192.168.2.50",
			},
			arpDevs: map[string]string{
				"AA:BB:CC:00:00:01": "phy1-sta0",
				"AA:BB:CC:00:00:02": "br-lan",
			},
		},
	}
	devs := l.buildDevices(polled, "")
	if len(devs) != 1 || devs[0].MAC != "AA:BB:CC:00:00:02" {
		t.Fatalf("esperaba solo el vecino de la LAN, got %+v", devs)
	}
}

// Un push event-driven (solo wireless + ARP, sin sección WAN) conserva las
// interfaces del uplink del último push completo: si no, sus vecinos salían
// como clientes cableados en cada asociación Wi-Fi.
func TestPolledFromAgentKeepsUplinkDevsOnEventPush(t *testing.T) {
	l := NewLive(nil, nil, []RouterConfig{{ID: "travel", Host: "192.0.2.3", Name: "travel", AgentOnly: true}}, nil)
	full := &probe.Payload{Router: "travel", Ts: 1, Version: "3.0.49",
		Data: probe.PayloadData{Wan: &probe.WanInfo{Proto: "dhcp", Device: "phy1-sta0", Port: "phy1-sta0"}}}
	_ = l.polledFromAgent(l.routers[0], full)
	event := &probe.Payload{Router: "travel", Ts: 2, Version: "3.0.49"}
	p := l.polledFromAgent(l.routers[0], event)
	if !arpUplinkDevs(p)["phy1-sta0"] {
		t.Fatalf("el push event-driven perdió el uplink: %+v", p.uplinkDevs)
	}
	if p.wanInfo.Proto != "" {
		t.Fatalf("wanInfo es dirección viva y no se recicla: %+v", p.wanInfo)
	}
}
