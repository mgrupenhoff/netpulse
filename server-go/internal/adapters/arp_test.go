// arp_test.go — Fallback de resolución IP vía tabla ARP (#377).
package adapters

import (
	"context"
	"testing"

	"github.com/gnacho/netpulse/agent/macaddr"
	"github.com/gnacho/netpulse/agent/probe"
	"github.com/gnacho/netpulse/server-go/internal/db"
)

func TestBuildDevicesArpFallback(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"patio": {
			cfg: RouterConfig{ID: "patio", Name: "Patio", Host: "192.168.1.2"},
			wireless: map[macaddr.MAC]WirelessClient{
				macaddr.MustParse("AA:BB:CC:DD:EE:FF"): {SignalDbm: -55, Band: "5 GHz"},
			},
			arp: map[macaddr.MAC]string{
				macaddr.MustParse("AA:BB:CC:DD:EE:FF"): "192.168.1.50",
			},
		},
	}
	devs := l.buildDevices(polled, "")
	if len(devs) != 1 {
		t.Fatalf("esperado 1 dispositivo, got %d: %+v", len(devs), devs)
	}
	if devs[0].IP != "192.168.1.50" {
		t.Fatalf("IP esperada 192.168.1.50, got %q (dispositivo %+v)", devs[0].IP, devs[0])
	}
}

// TestBuildDevicesArpDiscovery: una MAC visible SOLO por ARP (sin wireless,
// sin FDB, sin lease, sin device_attrib) aparece como dispositivo cableado
// online con la IP de la tabla ARP y el RouterID del router que la reportó
// (#507).
func TestBuildDevicesArpDiscovery(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"patio": {
			cfg: RouterConfig{ID: "patio", Name: "Patio", Host: "192.168.1.2"},
			arp: map[macaddr.MAC]string{
				macaddr.MustParse("AA:BB:CC:DD:EE:FF"): "192.168.1.60",
			},
		},
	}
	devs := l.buildDevices(polled, "")
	if len(devs) != 1 {
		t.Fatalf("esperado 1 dispositivo, got %d: %+v", len(devs), devs)
	}
	d := devs[0]
	if d.MAC != macaddr.MustParse("AA:BB:CC:DD:EE:FF") {
		t.Fatalf("MAC esperada AA:BB:CC:DD:EE:FF, got %q", d.MAC)
	}
	if d.IP != "192.168.1.60" {
		t.Fatalf("IP esperada 192.168.1.60, got %q", d.IP)
	}
	if d.Band != "cable" {
		t.Fatalf("Band esperada cable, got %q", d.Band)
	}
	if !d.Online {
		t.Fatalf("Online esperado true (presencia ARP = activo reciente)")
	}
	if d.RouterID != "patio" {
		t.Fatalf("RouterID esperado patio, got %q", d.RouterID)
	}
}

// TestBuildDevicesArpRouterExcluded: una MAC de bridge/router presente en la
// tabla ARP NO debe aparecer como dispositivo (#507).
func TestBuildDevicesArpRouterExcluded(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"patio": {
			cfg:   RouterConfig{ID: "patio", Name: "Patio", Host: "192.168.1.2"},
			brMac: macaddr.MustParse("AA:BB:CC:DD:EE:FF"),
			arp: map[macaddr.MAC]string{
				macaddr.MustParse("AA:BB:CC:DD:EE:FF"): "192.168.1.60",
			},
		},
	}
	devs := l.buildDevices(polled, "")
	if len(devs) != 0 {
		t.Fatalf("esperado 0 dispositivos (bridge MAC excluida), got %d: %+v", len(devs), devs)
	}
}

func TestPolledFromAgentArp(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	pl := &probe.Payload{Router: "patio", Version: "0.1.0"}
	pl.Data.System = &probe.SystemData{SysInfo: &probe.SysInfo{Uptime: 100}}
	pl.Data.Arp = map[string]string{"AA:BB:CC:DD:EE:FF": "192.168.1.50"}
	p := l.polledFromAgent(RouterConfig{ID: "patio"}, pl)
	if p == nil {
		t.Fatal("polledFromAgent devolvió nil")
	}
	if p.arp[macaddr.MustParse("AA:BB:CC:DD:EE:FF")] != "192.168.1.50" {
		t.Fatalf("arp no propagada: %+v", p.arp)
	}
}

// A MAC whose only evidence is a neighbour entry the kernel has NOT
// confirmed is not a device: the entry outlives the host by minutes, and
// with no port either the map hung it off the gateway bubble.
func TestBuildDevicesIgnoresStaleArpOnlyHosts(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"patio": {
			cfg: RouterConfig{ID: "patio", Name: "Patio", Host: "192.0.2.2"},
			arp: map[macaddr.MAC]string{
				macaddr.MustParse("02:00:00:00:00:10"): "192.0.2.60", // confirmed
				macaddr.MustParse("02:00:00:00:00:11"): "192.0.2.61", // only remembered
			},
			arpStale: map[macaddr.MAC]bool{macaddr.MustParse("02:00:00:00:00:11"): true},
		},
	}
	devs := l.buildDevices(polled, "")
	if len(devs) != 1 || devs[0].MAC != macaddr.MustParse("02:00:00:00:00:10") {
		t.Fatalf("only the confirmed host is a device: %+v", devs)
	}
}

// Stale is about presence, never about other evidence: a host with a lease
// stays, and its address still comes from the stale entry.
func TestBuildDevicesStaleArpStillResolvesIPs(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"patio": {
			cfg:      RouterConfig{ID: "patio", Name: "Patio", Host: "192.0.2.2"},
			wireless: map[macaddr.MAC]WirelessClient{macaddr.MustParse("02:00:00:00:00:11"): {SignalDbm: -55, Band: "5 GHz"}},
			arp:      map[macaddr.MAC]string{macaddr.MustParse("02:00:00:00:00:11"): "192.0.2.61"},
			arpStale: map[macaddr.MAC]bool{macaddr.MustParse("02:00:00:00:00:11"): true},
		},
	}
	devs := l.buildDevices(polled, "")
	if len(devs) != 1 || devs[0].IP != "192.0.2.61" {
		t.Fatalf("an associated client keeps its address: %+v", devs)
	}
}

// Two routers see the same host: confirmed anywhere is confirmed.
func TestBuildDevicesStaleOnOneRouterOnly(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"patio": {
			cfg:      RouterConfig{ID: "patio", Name: "Patio", Host: "192.0.2.2"},
			arp:      map[macaddr.MAC]string{macaddr.MustParse("02:00:00:00:00:10"): "192.0.2.60"},
			arpStale: map[macaddr.MAC]bool{macaddr.MustParse("02:00:00:00:00:10"): true},
		},
		"salon": {
			cfg: RouterConfig{ID: "salon", Name: "Salon", Host: "192.0.2.3"},
			arp: map[macaddr.MAC]string{macaddr.MustParse("02:00:00:00:00:10"): "192.0.2.60"},
		},
	}
	if devs := l.buildDevices(polled, ""); len(devs) != 1 {
		t.Fatalf("confirmed by one router is enough: %+v", devs)
	}
}

// A source that cannot report states (an older agent, or the /proc/net/arp
// fallback) marks nothing stale, and the whole table counts as presence.
func TestPolledFromAgentArpStale(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	pl := &probe.Payload{Router: "patio", Version: "0.1.0"}
	pl.Data.System = &probe.SystemData{SysInfo: &probe.SysInfo{Uptime: 100}}
	pl.Data.Arp = map[string]string{"02:00:00:00:00:10": "192.0.2.60"}
	if p := l.polledFromAgent(RouterConfig{ID: "patio"}, pl); len(p.arpStale) != 0 {
		t.Fatalf("an agent without arpStale marks nothing stale: %+v", p.arpStale)
	}

	pl.Data.ArpStale = []string{"02:00:00:00:00:10"}
	p := l.polledFromAgent(RouterConfig{ID: "patio"}, pl)
	if !p.arpStale[macaddr.MustParse("02:00:00:00:00:10")] {
		t.Fatalf("arpStale not propagated: %+v", p.arpStale)
	}
}

// A device_attrib row outlives the router it names: deleting a router, or an
// integration that registered one per AP, used to leave its clients
// attributed to an id nothing resolves, so no router counted them.
func TestBuildDevicesReattributesDeletedRouters(t *testing.T) {
	d := openLiveTestDB(t)
	l := NewLive(nil, d, nil, nil)
	l.SetRouters([]RouterConfig{
		{ID: "gateway", Name: "Gateway", Host: "192.0.2.1", IsGateway: true, AgentOnly: true},
	})
	// One client remembered on a router that still exists, one on a router
	// that was deleted.
	for _, r := range [][2]string{
		{"02:00:00:00:00:01", "gateway"},
		{"02:00:00:00:00:02", "ap-that-was-deleted"},
	} {
		if _, err := d.Exec(
			`INSERT INTO device_attrib (mac, router_id, band, signal_dbm, last_seen) VALUES (?,?,?,?,?)`,
			r[0], r[1], "5 GHz", -50, db.NowMS()); err != nil {
			t.Fatal(err)
		}
	}
	polled := map[string]*routerPolled{
		"gateway": {
			cfg: RouterConfig{ID: "gateway", Name: "Gateway", Host: "192.0.2.1", IsGateway: true},
			leases: []DhcpLease{
				{MAC: macaddr.MustParse("02:00:00:00:00:01"), IP: "192.0.2.51", Hostname: "one"},
				{MAC: macaddr.MustParse("02:00:00:00:00:02"), IP: "192.0.2.52", Hostname: "two"},
			},
		},
	}
	byMAC := map[macaddr.MAC]Device{}
	for _, dev := range l.buildDevices(polled, "") {
		byMAC[dev.MAC] = dev
	}
	if len(byMAC) != 2 {
		t.Fatalf("devices: %+v", byMAC)
	}
	for mac, dev := range byMAC {
		if dev.RouterID != "gateway" {
			t.Fatalf("%s should fall back to the gateway, got %q", mac, dev.RouterID)
		}
	}
}

// countClientsPerRouter: online devices attributed to each router, and
// nothing else -- offline devices and devices attributed to a router that is
// not in the list do not count anywhere.
func TestCountClientsPerRouter(t *testing.T) {
	routers := []Router{{ID: "gateway"}, {ID: "patio"}}
	devices := []Device{
		{MAC: macaddr.MustParse("02:00:00:00:00:01"), RouterID: "gateway", Band: "cable", Online: true},
		{MAC: macaddr.MustParse("02:00:00:00:00:02"), RouterID: "gateway", Band: "5 GHz", Online: true},
		{MAC: macaddr.MustParse("02:00:00:00:00:03"), RouterID: "gateway", Band: "2.4 GHz", Online: false},
		{MAC: macaddr.MustParse("02:00:00:00:00:04"), RouterID: "patio", Band: "2.4 GHz", Online: true},
		{MAC: macaddr.MustParse("02:00:00:00:00:05"), RouterID: "ghost", Band: "cable", Online: true},
	}
	countClientsPerRouter(routers, devices)

	if routers[0].Clients != 2 || routers[1].Clients != 1 {
		t.Fatalf("counts: %d %d", routers[0].Clients, routers[1].Clients)
	}
	if routers[0].BandSplit == nil || routers[0].BandSplit.Cable != 1 || routers[0].BandSplit.Band5 != 1 {
		t.Fatalf("band split: %+v", routers[0].BandSplit)
	}
	// The offline 2.4 GHz device is not in the split either.
	if routers[0].BandSplit.Band24 != 0 {
		t.Fatalf("an offline device was counted: %+v", routers[0].BandSplit)
	}
}

// GET /api/routers used to report the DHCP lease count while the overview
// reported the real attribution, so the routers page and the map disagreed
// about the same router.
func TestGetRoutersCountsClientsNotLeases(t *testing.T) {
	d := openLiveTestDB(t)
	l := NewLive(nil, d, nil, nil)
	cfg := RouterConfig{ID: "gateway", Name: "Gateway", Host: "192.0.2.1", IsGateway: true, AgentOnly: true}
	l.SetRouters([]RouterConfig{cfg})

	// #1818: un cliente nuevo se retiene un minuto antes de listarse. Estos dos
	// ya estaban vistos, que es lo que este test mide (leases vs asociados).
	l.noteDevicesSeen([]Device{
		{MAC: macaddr.MustParse("02:00:00:00:00:01"), Online: true, Hostname: "one"},
		{MAC: macaddr.MustParse("02:00:00:00:00:02"), Online: true, Hostname: "two"},
	}, 1000)

	// Three leases, but only two of those hosts are associated right now.
	l.mu.Lock()
	l.lastPolled = map[string]*routerPolled{"gateway": {
		cfg: cfg,
		leases: []DhcpLease{
			{MAC: macaddr.MustParse("02:00:00:00:00:01"), IP: "192.0.2.51", Hostname: "one"},
			{MAC: macaddr.MustParse("02:00:00:00:00:02"), IP: "192.0.2.52", Hostname: "two"},
			{MAC: macaddr.MustParse("02:00:00:00:00:03"), IP: "192.0.2.53", Hostname: "gone"},
		},
		wireless: map[macaddr.MAC]WirelessClient{
			macaddr.MustParse("02:00:00:00:00:01"): {SignalDbm: -50, Band: "5 GHz"},
			macaddr.MustParse("02:00:00:00:00:02"): {SignalDbm: -60, Band: "2.4 GHz"},
		},
	}}
	l.mu.Unlock()

	routers := l.GetRouters(context.Background())
	if len(routers) != 1 {
		t.Fatalf("routers: %+v", routers)
	}
	if routers[0].Clients != 2 {
		t.Fatalf("clients: got %d, want 2 (the associated ones, not the 3 leases)", routers[0].Clients)
	}
	// And the same number the overview would report.
	online := 0
	for _, dev := range l.attributedDevices() {
		if dev.Online && dev.RouterID == "gateway" {
			online++
		}
	}
	if online != routers[0].Clients {
		t.Fatalf("routers endpoint says %d, attribution says %d", routers[0].Clients, online)
	}
}

// #1485: el ARP aprendido en el puerto WAN del gateway cuenta como presencia
// para el primer salto del enlace (ONT/modem en bridge tras PPPoE), que no
// está en ninguna FDB. Solo en la boca WAN: el ARP de una boca LAN no se marca.
func TestBuildDevicesWanSideArp(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	l.gatewayCfg = &RouterConfig{ID: "gw", Name: "Gateway", IsGateway: true}
	polled := map[string]*routerPolled{
		"gw": {
			cfg: RouterConfig{ID: "gw", Name: "Gateway", IsGateway: true},
			arp: map[macaddr.MAC]string{
				macaddr.MustParse("C0:49:43:A3:D1:34"): "192.168.1.1",  // ONT en el puerto WAN
				macaddr.MustParse("AA:BB:CC:00:00:01"): "192.168.1.60", // cliente visto por LAN
			},
			arpDevs: map[macaddr.MAC]string{
				macaddr.MustParse("C0:49:43:A3:D1:34"): "eth2",
				macaddr.MustParse("AA:BB:CC:00:00:01"): "br-lan",
			},
		},
	}
	devs := l.buildDevices(polled, "eth2")
	byMAC := map[macaddr.MAC]Device{}
	for _, d := range devs {
		byMAC[d.MAC] = d
	}
	ont, ok := byMAC[macaddr.MustParse("C0:49:43:A3:D1:34")]
	if !ok {
		t.Fatalf("el ONT no aparece en devices: %+v", devs)
	}
	if !ont.WanSide {
		t.Fatalf("ONT: %+v, want WanSide (ARP en el puerto WAN)", ont)
	}
	if !ont.Online {
		t.Fatalf("ONT: %+v, want online (el ARP del puerto WAN es presencia)", ont)
	}
	lan, ok := byMAC[macaddr.MustParse("AA:BB:CC:00:00:01")]
	if !ok {
		t.Fatalf("el cliente LAN no aparece: %+v", devs)
	}
	if lan.WanSide {
		t.Fatalf("un ARP de boca LAN no puede ser WanSide: %+v", lan)
	}
	// Sin boca WAN (p.ej. rutas que no la pasan) no se marca nada.
	for _, d := range l.buildDevices(polled, "") {
		if d.WanSide {
			t.Fatalf("sin wanPort no debe haber WanSide: %+v", d)
		}
	}
}

// #1489: la MAC que responde por la IP de gestión de una unidad de flota es la
// propia unidad (pila de mgmt del switch), no un cliente: se absorbe como las
// MACs de bridge aunque el FDB vecino la aprenda en su br-lan.
func TestBuildDevicesAbsorbsSwitchMgmtMac(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	l.routers = []RouterConfig{{ID: "xs708t", Host: "192.168.2.6", Type: "managed-switch"}}
	polled := map[string]*routerPolled{
		"gw": {
			cfg: RouterConfig{ID: "gw", Host: "192.168.1.1", IsGateway: true},
			arp: map[macaddr.MAC]string{
				macaddr.MustParse("08:BD:43:75:50:CD"): "192.168.2.6", // el propio switch
				macaddr.MustParse("AA:BB:CC:00:00:09"): "192.168.2.9", // cliente normal
			},
		},
	}
	devs := l.buildDevices(polled, "")
	for _, d := range devs {
		if d.MAC == macaddr.MustParse("08:BD:43:75:50:CD") {
			t.Fatalf("la MAC de gestión del switch no es un cliente: %+v", d)
		}
	}
	if len(devs) != 1 || devs[0].MAC != macaddr.MustParse("AA:BB:CC:00:00:09") {
		t.Fatalf("devices: %+v, want solo el cliente normal", devs)
	}
}

// #2183: en una WAN con subred pública compartida el ARP del puerto WAN
// trae decenas de vecinos del ISP. Solo el peer de la ruta por defecto es
// "lado WAN"; el resto se absorbe (para no atribuirlo a una boca LAN) pero
// no se materializa como dispositivo del inventario.
func TestBuildDevicesWanSideOnlyDefaultRoutePeer(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	l.gatewayCfg = &RouterConfig{ID: "gw", Name: "Gateway", IsGateway: true}
	polled := map[string]*routerPolled{
		"gw": {
			cfg:     RouterConfig{ID: "gw", Name: "Gateway", IsGateway: true},
			wanInfo: probe.WanInfo{Gateway: "203.0.113.1"},
			arp: map[macaddr.MAC]string{
				macaddr.MustParse("0C:EF:15:00:00:01"): "203.0.113.1",   // peer de la ruta por defecto
				macaddr.MustParse("0C:EF:15:00:00:02"): "203.0.113.248", // vecino del ISP
				macaddr.MustParse("AA:BB:CC:00:00:01"): "192.168.1.60",  // cliente de la LAN
			},
			arpDevs: map[macaddr.MAC]string{
				macaddr.MustParse("0C:EF:15:00:00:01"): "eth1",
				macaddr.MustParse("0C:EF:15:00:00:02"): "eth1",
				macaddr.MustParse("AA:BB:CC:00:00:01"): "br-lan",
			},
		},
	}
	devs := l.buildDevices(polled, "eth1")
	byMAC := map[macaddr.MAC]Device{}
	for _, d := range devs {
		byMAC[d.MAC] = d
	}
	if _, ok := byMAC[macaddr.MustParse("0C:EF:15:00:00:02")]; ok {
		t.Fatalf("el vecino del ISP no puede ser un dispositivo del inventario: %+v", byMAC[macaddr.MustParse("0C:EF:15:00:00:02")])
	}
	lan, ok := byMAC[macaddr.MustParse("AA:BB:CC:00:00:01")]
	if !ok || lan.WanSide {
		t.Fatalf("el cliente LAN debe seguir: %+v", lan)
	}
	// Gate negativo: sin la IP de la puerta de enlace WAN no hay forma de
	// distinguir el peer, así que se conserva el comportamiento anterior.
	delete(polled["gw"].arp, macaddr.MustParse("AA:BB:CC:00:00:01"))
	polled["gw"].wanInfo = probe.WanInfo{}
	for _, d := range l.buildDevices(polled, "eth1") {
		if d.MAC != macaddr.MustParse("0C:EF:15:00:00:02") {
			continue
		}
		if !d.WanSide {
			t.Fatalf("sin gateway WAN se conserva el peer marcado WanSide: %+v", d)
		}
	}
}

// Un vecino aprendido en el uplink de una unidad que no es el gateway (un
// router de viaje colgado de una Wi-Fi ajena) es un equipo de esa red, no un
// cliente: antes salía como cableado desconocido y disparaba su alerta. Los
// vecinos de su LAN siguen apareciendo.
func TestBuildDevicesSkipsArpOnUplink(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"travel": {
			cfg:     RouterConfig{ID: "travel", Name: "travel", Host: "192.168.2.1"},
			wanInfo: probe.WanInfo{Proto: "dhcp", Device: "phy1-sta0", Port: "phy1-sta0"},
			arp: map[macaddr.MAC]string{
				macaddr.MustParse("AA:BB:CC:00:00:01"): "10.20.0.177",
				macaddr.MustParse("AA:BB:CC:00:00:02"): "192.168.2.50",
			},
			arpDevs: map[macaddr.MAC]string{
				macaddr.MustParse("AA:BB:CC:00:00:01"): "phy1-sta0",
				macaddr.MustParse("AA:BB:CC:00:00:02"): "br-lan",
			},
		},
	}
	devs := l.buildDevices(polled, "")
	if len(devs) != 1 || devs[0].MAC != macaddr.MustParse("AA:BB:CC:00:00:02") {
		t.Fatalf("esperaba solo el vecino de la LAN, got %+v", devs)
	}
}

// En un AP el uplink es la propia LAN (br-lan con DHCP cliente): sus vecinos
// sí son clientes y no se descartan.
func TestBuildDevicesKeepsArpOnBridgeUplink(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"ap": {
			cfg:     RouterConfig{ID: "ap", Name: "AP", Host: "192.168.2.3"},
			wanInfo: probe.WanInfo{Proto: "dhcp", Device: "br-lan", Port: "br-lan"},
			arp:     map[macaddr.MAC]string{macaddr.MustParse("AA:BB:CC:00:00:03"): "192.168.2.60"},
			arpDevs: map[macaddr.MAC]string{macaddr.MustParse("AA:BB:CC:00:00:03"): "br-lan"},
		},
	}
	if devs := l.buildDevices(polled, ""); len(devs) != 1 {
		t.Fatalf("el vecino de la LAN de un AP debe aparecer, got %+v", devs)
	}
}

// #1485 en las tres vías: attributeDevices saca la boca WAN del sondeo del
// gateway. GetRouterDetail pasaba "" y la ONT bridged salía en los clientes
// del gateway sin WanSide; el overview y /api/devices ya la pasaban.
func TestAttributeDevicesWanSideArp(t *testing.T) {
	l := NewLive(nil, nil, nil, nil)
	cfg := RouterConfig{ID: "gw", Name: "Gateway", IsGateway: true}
	l.gatewayCfg = &cfg
	ont := macaddr.MustParse("C0:49:43:A3:D1:34")
	p := &routerPolled{
		cfg:     cfg,
		arp:     map[macaddr.MAC]string{ont: "192.168.1.1"},
		arpDevs: map[macaddr.MAC]string{ont: "eth2"},
	}
	p.wanInfo.Port = "eth2"

	_, devs, _ := l.attributeDevices(map[string]*routerPolled{"gw": p}, nil, nil, 1000)
	for _, d := range devs {
		if d.MAC == ont {
			if !d.WanSide {
				t.Fatalf("ONT: %+v, esperaba WanSide (ARP en la boca WAN del gateway)", d)
			}
			return
		}
	}
	t.Fatalf("la ONT no aparece en devices: %+v", devs)
}
