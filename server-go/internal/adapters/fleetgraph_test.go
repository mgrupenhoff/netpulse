package adapters

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/gnacho/netpulse/agent/macaddr"
	"github.com/gnacho/netpulse/server-go/internal/db"
)

var (
	fgMacGW     = macaddr.MustParse("AA:BB:CC:00:00:01")
	fgMacBase   = macaddr.MustParse("AA:BB:CC:00:00:02")
	fgMacSat    = macaddr.MustParse("AA:BB:CC:00:00:03")
	fgMacPi     = macaddr.MustParse("10:00:00:00:00:01")
	fgMacTV     = macaddr.MustParse("10:00:00:00:00:02")
	fgMacDirect = macaddr.MustParse("10:00:00:00:00:03")
	fgMacSatCli = macaddr.MustParse("10:00:00:00:00:04")
	fgMacGwCli  = macaddr.MustParse("10:00:00:00:00:05")
)

func scenarioARouters() []RouterConfig {
	return []RouterConfig{
		{ID: "gateway", Name: "Gateway", IsGateway: true},
		{ID: "base", Name: "Base"},
		{ID: "sat", Name: "Sat"},
	}
}

func scenarioAPolled() map[string]*routerPolled {
	return map[string]*routerPolled{
		"gateway": {
			cfg:   RouterConfig{ID: "gateway", Name: "Gateway", IsGateway: true},
			brMac: fgMacGW,
			fdb: map[macaddr.MAC]string{
				fgMacGW:     "lan1",
				fgMacGwCli:  "lan1",
				fgMacBase:   "lan2",
				fgMacSat:    "lan2",
				fgMacDirect: "lan2",
				fgMacPi:     "lan2",
				fgMacTV:     "lan2",
				fgMacSatCli: "lan2",
			},
		},
		"base": {
			cfg:   RouterConfig{ID: "base", Name: "Base"},
			brMac: fgMacBase,
			fdb: map[macaddr.MAC]string{
				fgMacGW:     "lan1",
				fgMacGwCli:  "lan1",
				fgMacSat:    "lan3",
				fgMacPi:     "lan3",
				fgMacTV:     "lan3",
				fgMacSatCli: "lan3",
				fgMacDirect: "lan4",
			},
		},
		"sat": {
			cfg:   RouterConfig{ID: "sat", Name: "Sat"},
			brMac: fgMacSat,
			fdb: map[macaddr.MAC]string{
				fgMacGW:     "lan1",
				fgMacBase:   "lan1",
				fgMacGwCli:  "lan1",
				fgMacDirect: "lan1",
				fgMacPi:     "lan1",
				fgMacTV:     "lan1",
				fgMacSatCli: "lan2",
			},
		},
	}
}

func scenarioBPolled() map[string]*routerPolled {
	p := scenarioAPolled()
	p["base"].cfg = RouterConfig{ID: "base", Name: "Base", Type: "external", AgentOnly: true}
	delete(p["base"].fdb, fgMacGW)
	delete(p["base"].fdb, fgMacGwCli)
	p["sat"].cfg = RouterConfig{ID: "sat", Name: "Sat", Type: "external", AgentOnly: true}
	p["sat"].fdb = map[macaddr.MAC]string{fgMacSatCli: "lan2"}
	return p
}

func routersFromPolled(polled map[string]*routerPolled) []RouterConfig {
	ids := make([]string, 0, len(polled))
	for id := range polled {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]RouterConfig, 0, len(ids))
	for _, id := range ids {
		out = append(out, polled[id].cfg)
	}
	return out
}

func runFleetGraphTick(l *Live, polled map[string]*routerPolled, now int64) ([]Device, []DistributionNode) {
	_, devs, dists := l.attributeDevices(polled, nil, nil, now)
	return devs, dists
}

type wantDevAttr struct {
	routerID string
	port     string
	attachTo string
}

func TestFleetGraphAttribution(t *testing.T) {
	var (
		macSw1 = macaddr.MustParse("AA:BB:CC:00:00:11")
		macSw2 = macaddr.MustParse("AA:BB:CC:00:00:12")
		macAP  = macaddr.MustParse("AA:BB:CC:00:00:13")
		macMid = macaddr.MustParse("10:00:00:00:00:30")
	)

	wantScenarioAB := map[macaddr.MAC]wantDevAttr{
		fgMacPi:     {routerID: "base", port: "lan3", attachTo: "dist-base-lan3"},
		fgMacTV:     {routerID: "base", port: "lan3", attachTo: "dist-base-lan3"},
		fgMacDirect: {routerID: "base", port: "lan4", attachTo: ""},
		fgMacSatCli: {routerID: "sat", port: "lan2", attachTo: ""},
		fgMacGwCli:  {routerID: "gateway", port: "lan1", attachTo: ""},
	}

	nilSatPolled := scenarioAPolled()
	nilSatPolled["sat"].fdb = nil

	noLan3Polled := scenarioBPolled()
	noLan3Polled["base"].fdb = map[macaddr.MAC]string{fgMacDirect: "lan4"}

	tests := []struct {
		name        string
		polled      map[string]*routerPolled
		wantDevs    map[macaddr.MAC]wantDevAttr
		wantAbsent  []macaddr.MAC
		wantDists   map[string]int
		forbidDists []string
	}{
		{
			name:        "escenario A tablas crudas",
			polled:      scenarioAPolled(),
			wantDevs:    wantScenarioAB,
			wantAbsent:  []macaddr.MAC{fgMacGW, fgMacBase, fgMacSat},
			wantDists:   map[string]int{"dist-base-lan3": 2},
			forbidDists: []string{"dist-sat-lan1", "dist-base-lan1"},
		},
		{
			name:        "escenario B pusher prefiltrado sin puertos raíz",
			polled:      scenarioBPolled(),
			wantDevs:    wantScenarioAB,
			wantAbsent:  []macaddr.MAC{fgMacGW, fgMacBase, fgMacSat},
			wantDists:   map[string]int{"dist-base-lan3": 2},
			forbidDists: []string{"dist-sat-lan1", "dist-base-lan1"},
		},
		{
			name: "tres saltos gw -> sw1 -> sw2 -> ap",
			polled: map[string]*routerPolled{
				"gateway": {
					cfg:   RouterConfig{ID: "gateway", IsGateway: true},
					brMac: fgMacGW,
					fdb:   map[macaddr.MAC]string{macSw1: "lan2", macSw2: "lan2", macAP: "lan2", macMid: "lan2"},
				},
				"sw1": {
					cfg:   RouterConfig{ID: "sw1"},
					brMac: macSw1,
					fdb:   map[macaddr.MAC]string{fgMacGW: "lan1", macSw2: "lan3", macAP: "lan3", macMid: "lan3"},
				},
				"sw2": {
					cfg:   RouterConfig{ID: "sw2"},
					brMac: macSw2,
					fdb:   map[macaddr.MAC]string{fgMacGW: "lan1", macSw1: "lan1", macMid: "lan1", macAP: "lan2"},
				},
				"ap": {
					cfg:   RouterConfig{ID: "ap"},
					brMac: macAP,
					fdb:   map[macaddr.MAC]string{fgMacGW: "lan1", macSw1: "lan1", macSw2: "lan1"},
				},
			},
			wantDevs:   map[macaddr.MAC]wantDevAttr{macMid: {routerID: "sw1", port: "lan3"}},
			wantAbsent: []macaddr.MAC{fgMacGW, macSw1, macSw2, macAP},
		},
		{
			name: "switch no gestionado en puerto del gateway con sat detrás",
			polled: map[string]*routerPolled{
				"gateway": {
					cfg:   RouterConfig{ID: "gateway", IsGateway: true},
					brMac: fgMacGW,
					fdb:   map[macaddr.MAC]string{fgMacSat: "lan2", fgMacPi: "lan2", fgMacTV: "lan2", fgMacSatCli: "lan2"},
				},
				"sat": {
					cfg:   RouterConfig{ID: "sat"},
					brMac: fgMacSat,
					fdb:   map[macaddr.MAC]string{fgMacGW: "lan1", fgMacPi: "lan1", fgMacTV: "lan1", fgMacSatCli: "lan2"},
				},
			},
			wantDevs: map[macaddr.MAC]wantDevAttr{
				fgMacPi:     {routerID: "gateway", port: "lan2", attachTo: "dist-gateway-lan2"},
				fgMacTV:     {routerID: "gateway", port: "lan2", attachTo: "dist-gateway-lan2"},
				fgMacSatCli: {routerID: "sat", port: "lan2"},
			},
			wantAbsent:  []macaddr.MAC{fgMacGW, fgMacSat},
			wantDists:   map[string]int{"dist-gateway-lan2": 2},
			forbidDists: []string{"dist-sat-lan1"},
		},
		{
			name:   "hijo con fdb nil bloquea los residuos interiores por encima",
			polled: nilSatPolled,
			wantDevs: map[macaddr.MAC]wantDevAttr{
				fgMacDirect: {routerID: "base", port: "lan4"},
				fgMacGwCli:  {routerID: "gateway", port: "lan1"},
			},
			wantAbsent: []macaddr.MAC{fgMacPi, fgMacTV, fgMacSatCli, fgMacGW, fgMacBase, fgMacSat},
		},
		{
			name:   "sin puerto de bajada cae al gateway",
			polled: noLan3Polled,
			wantDevs: map[macaddr.MAC]wantDevAttr{
				fgMacPi:     {routerID: "gateway", port: "lan2", attachTo: "dist-gateway-lan2"},
				fgMacTV:     {routerID: "gateway", port: "lan2", attachTo: "dist-gateway-lan2"},
				fgMacDirect: {routerID: "base", port: "lan4"},
				fgMacSatCli: {routerID: "sat", port: "lan2"},
				fgMacGwCli:  {routerID: "gateway", port: "lan1"},
			},
			wantAbsent: []macaddr.MAC{fgMacGW, fgMacBase, fgMacSat},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLive(nil, nil, routersFromPolled(tt.polled), nil)
			devs, dists := runFleetGraphTick(l, tt.polled, 1_700_000_000_000)

			byMAC := map[macaddr.MAC]Device{}
			for _, d := range devs {
				byMAC[d.MAC] = d
			}
			for _, absent := range tt.wantAbsent {
				if d, ok := byMAC[absent]; ok {
					t.Errorf("la MAC %s no debería estar como dispositivo, obtuve %+v", absent, d)
				}
			}
			for mac, want := range tt.wantDevs {
				d, ok := byMAC[mac]
				if !ok {
					t.Errorf("no se encontró el dispositivo esperado %s en %+v", mac, devs)
					continue
				}
				if d.RouterID != want.routerID || d.Port != want.port || d.AttachTo != want.attachTo {
					t.Errorf("dispositivo %s = {RouterID:%q Port:%q AttachTo:%q}, esperaba %+v",
						mac, d.RouterID, d.Port, d.AttachTo, want)
				}
			}
			distByID := map[string]DistributionNode{}
			for _, dn := range dists {
				distByID[dn.ID] = dn
			}
			for id, wantCount := range tt.wantDists {
				dn, ok := distByID[id]
				if !ok {
					t.Errorf("no se encontró el distnode esperado %s en %+v", id, dists)
					continue
				}
				if dn.Kind != "inferred" || dn.MacCount != wantCount {
					t.Errorf("distnode %s = %+v, esperaba Kind=inferred MacCount=%d", id, dn, wantCount)
				}
			}
			for _, forbid := range tt.forbidDists {
				if dn, ok := distByID[forbid]; ok {
					t.Errorf("distnode inesperado %s: %+v", forbid, dn)
				}
			}
		})
	}
}

func TestFleetGraphChildBrMacAgesOutKeepsDownstreamRole(t *testing.T) {
	l := NewLive(nil, nil, scenarioARouters(), nil)
	now := int64(1_700_000_000_000)

	polled1 := scenarioAPolled()
	l.updateFdbMemo(polled1, now)

	polled2 := scenarioAPolled()
	delete(polled2["base"].fdb, fgMacSat)

	fg2 := l.buildFleetGraph(polled2, nil, nil, now+10_000)
	if fg2.roles.role("base", "lan3") != roleDown {
		t.Fatalf("base/lan3 debería seguir de bajada vía portRoleMemo, obtuve %+v", fg2.roles["base"])
	}
	devs2, _ := runFleetGraphTick(l, polled2, now+10_000)

	pi := mustDevice(t, devs2, fgMacPi.String())
	if pi.RouterID != "base" || pi.Port != "lan3" {
		t.Fatalf("Pi = {RouterID:%q Port:%q}, esperaba base/lan3", pi.RouterID, pi.Port)
	}
}

func TestFleetGraphKnownWirelessNotClaimedByInteriorPort(t *testing.T) {
	d := openLiveTestDB(t)
	l := NewLive(nil, d, scenarioARouters(), nil)
	roamingWifi := macaddr.MustParse("10:00:00:00:00:99")
	if _, err := d.Exec(
		`INSERT INTO device_attrib (mac, router_id, band, signal_dbm, last_seen) VALUES (?, ?, ?, ?, ?)`,
		roamingWifi.String(), "sat", "5 GHz", -58, db.NowMS(),
	); err != nil {
		t.Fatalf("sembrar device_attrib: %v", err)
	}

	polled := scenarioAPolled()
	polled["base"].fdb[roamingWifi] = "lan3"
	polled["gateway"].fdb[roamingWifi] = "lan2"

	for _, dev := range l.buildDevices(polled, "") {
		if dev.MAC == roamingWifi {
			if dev.RouterID == "base" || dev.Band == "cable" || dev.Online {
				t.Fatalf("un cliente wireless conocido no debería reclamarse como cableado online en base/lan3: %+v", dev)
			}
		}
	}
}

func TestFleetGraphLeafMemoPrecedenceOverInterior(t *testing.T) {
	l := NewLive(nil, nil, scenarioARouters(), nil)
	t0 := int64(1_700_000_000_000)

	polled1 := scenarioAPolled()
	memo1 := l.updateFdbMemo(polled1, t0)
	if m := memo1[fgMacSatCli]; m.routerID != "sat" || m.port != "lan2" || !m.leaf {
		t.Fatalf("memo del tick 1 = %+v, esperaba sat/lan2 leaf=true", m)
	}

	polled2 := scenarioAPolled()
	delete(polled2["sat"].fdb, fgMacSatCli)
	memo2 := l.updateFdbMemo(polled2, t0+5*60_000)
	if m := memo2[fgMacSatCli]; m.routerID != "sat" || m.port != "lan2" || !m.leaf {
		t.Fatalf("dentro del TTL, la memo hoja debe sobrevivir a un avistamiento interior: obtuve %+v", m)
	}

	memo3 := l.updateFdbMemo(polled2, t0+fdbStickyTTL.Milliseconds()+60_000)
	if m := memo3[fgMacSatCli]; m.routerID != "base" || m.port != "lan3" || m.leaf {
		t.Fatalf("tras el TTL, el avistamiento interior debería sustituir la memo hoja caducada: obtuve %+v", m)
	}
}

func TestFleetGraphResolveWiredDeterminism(t *testing.T) {
	var (
		macA   = macaddr.MustParse("AA:BB:CC:00:00:A1")
		macB   = macaddr.MustParse("AA:BB:CC:00:00:B1")
		macTie = macaddr.MustParse("10:00:00:00:00:77")
	)
	polled := map[string]*routerPolled{
		"gateway": {
			cfg:   RouterConfig{ID: "gateway", IsGateway: true},
			brMac: fgMacGW,
			fdb:   map[macaddr.MAC]string{macA: "lan1", macB: "lan2", macTie: "lan1"},
		},
		"apA": {
			cfg:   RouterConfig{ID: "apA"},
			brMac: macA,
			fdb:   map[macaddr.MAC]string{fgMacGW: "lan1", macTie: "lan3"},
		},
		"apB": {
			cfg:   RouterConfig{ID: "apB"},
			brMac: macB,
			fdb:   map[macaddr.MAC]string{fgMacGW: "lan1", macTie: "lan2"},
		},
	}
	l := NewLive(nil, nil, routersFromPolled(polled), nil)
	fg := l.buildFleetGraph(polled, nil, nil, int64(1_700_000_000_000))
	first := resolveWiredFdb(polled, fg)
	want := fdbAttachment{routerID: "apA", port: "lan3", isLeaf: true, depth: 1}
	if got := first[macTie]; got != want {
		t.Fatalf("resolución inicial = %+v, esperaba %+v", got, want)
	}
	for i := 0; i < 50; i++ {
		if got := resolveWiredFdb(polled, fg); !reflect.DeepEqual(got, first) {
			t.Fatalf("la ejecución %d difiere: obtuve %+v, primera %+v", i, got, first)
		}
	}
}

func TestFleetGraphEmptyMatchesLegacy(t *testing.T) {
	polled := map[string]*routerPolled{
		"flint2": {cfg: RouterConfig{ID: "flint2", IsGateway: true}, brMac: macaddr.MustParse("94:83:C4:00:00:01"), fdb: map[macaddr.MAC]string{
			macaddr.MustParse("78:2B:CB:AA:01:01"): "lan3",
			macaddr.MustParse("8C:EA:48:AA:02:02"): "lan3",
		}},
		"rt2": {cfg: RouterConfig{ID: "rt2"}, brMac: macaddr.MustParse("94:83:C4:00:00:02"), fdb: map[macaddr.MAC]string{
			macaddr.MustParse("3C:D9:2B:AA:03:03"): "lan2",
			macaddr.MustParse("50:EC:50:AA:04:04"): "lan2",
		}},
	}
	devices := []Device{
		dev("78:2B:CB:AA:01:01", "flint2", "cable"),
		dev("8C:EA:48:AA:02:02", "flint2", "cable"),
		dev("3C:D9:2B:AA:03:03", "rt2", "cable"),
		dev("50:EC:50:AA:04:04", "rt2", "cable"),
	}
	devices, dists := inferTopologyWithGraph(polled, devices, &fleetGraph{})
	if len(dists) != 1 || dists[0].ID != "dist-flint2-lan3" || dists[0].Kind != "inferred" {
		t.Fatalf("con un fg vacío esperaba solo el distnode inferido del gateway, obtuve %+v", dists)
	}
	for _, d := range devices[2:] {
		if d.Port != "" || d.AttachTo != "" {
			t.Fatalf("cliente del AP modificado con un fg vacío: %s port=%q attachTo=%q", d.MAC, d.Port, d.AttachTo)
		}
	}
}

func TestFleetGraphCousinMACKeepsUplinkRoot(t *testing.T) {
	fgMacSat2 := macaddr.MustParse("AA:BB:CC:00:00:04")
	routers := append(scenarioARouters(), RouterConfig{ID: "sat2", Name: "Sat2"})
	withSat2 := func() map[string]*routerPolled {
		p := scenarioAPolled()
		p["gateway"].fdb[fgMacSat2] = "lan3"
		p["sat"].fdb[fgMacSat2] = "lan1" // tránsito por el uplink de sat
		p["sat2"] = &routerPolled{
			cfg:   RouterConfig{ID: "sat2", Name: "Sat2"},
			brMac: fgMacSat2,
			fdb:   map[macaddr.MAC]string{fgMacGW: "lan1"},
		}
		return p
	}
	l := NewLive(nil, nil, routers, nil)
	now := int64(1_700_000_000_000)

	l.updateFdbMemo(withSat2(), now)

	polled2 := withSat2()
	delete(polled2["sat"].fdb, fgMacGW)
	delete(polled2["sat"].fdb, fgMacBase)
	fg2 := l.buildFleetGraph(polled2, nil, nil, now+10_000)
	if got := fg2.roles.role("sat", "lan1"); got != roleRoot {
		t.Fatalf("sat/lan1: role=%v, esperaba raíz (%v)", got, roleRoot)
	}
	devs := l.buildDevicesWithGraph(polled2, fg2, "")
	for _, mac := range []macaddr.MAC{fgMacPi, fgMacTV} {
		if d := mustDevice(t, devs, mac.String()); d.RouterID != "base" {
			t.Errorf("%s: RouterID=%q, esperaba base", mac, d.RouterID)
		}
	}
}

// recabledAPPolled: un AP colgado del puerto apPort del gateway; pcOnAP decide
// si el PC va cableado a la lan3 del AP (true) o a la lan4 del gateway.
func recabledAPPolled(apPort string, pcOnAP bool) map[string]*routerPolled {
	gwFdb := map[macaddr.MAC]string{fgMacGW: "lan1", fgMacBase: apPort}
	apFdb := map[macaddr.MAC]string{fgMacBase: "lan1"}
	if pcOnAP {
		gwFdb[fgMacPi] = apPort
		apFdb[fgMacPi] = "lan3"
	} else {
		gwFdb[fgMacPi] = "lan4"
	}
	return map[string]*routerPolled{
		"gateway": {cfg: RouterConfig{ID: "gateway", Name: "GW", IsGateway: true}, brMac: fgMacGW, fdb: gwFdb,
			arp: map[macaddr.MAC]string{fgMacPi: "192.168.1.50"}},
		"ap": {cfg: RouterConfig{ID: "ap", Name: "AP"}, brMac: fgMacBase, fdb: apFdb},
	}
}

// TestFleetGraphDownstreamRoleExpires: el AP pasa de la lan4 a la lan2 del
// gateway y el PC que colgaba del AP se enchufa en la lan4 liberada. Cuando el
// rol de bajada de lan4 lleva portRoleTTL sin evidencia, lo que el gateway ve
// ahí vuelve a ser local y gana a la memo que aún sitúa el PC en el AP. El
// paso 5 lee el reloj real, así que los ticks se fechan contra él.
func TestFleetGraphDownstreamRoleExpires(t *testing.T) {
	polled1 := recabledAPPolled("lan4", true)
	l := NewLive(nil, nil, routersFromPolled(polled1), nil)
	now := time.Now().UnixMilli()
	t0 := now - portRoleTTL.Milliseconds() - 60_000
	runFleetGraphTick(l, polled1, t0)

	polled2 := recabledAPPolled("lan2", false)
	early := l.buildFleetGraph(polled2, nil, nil, t0+60_000)
	if got := early.roles.role("gateway", "lan4"); got != roleDown {
		t.Fatalf("gateway/lan4 un minuto tras el cambio: role=%v, esperaba aún de bajada (%v)", got, roleDown)
	}

	devs, _ := runFleetGraphTick(l, polled2, now)
	fg := l.buildFleetGraph(polled2, nil, nil, now)
	if got := fg.roles.role("gateway", "lan4"); got != roleLeaf {
		t.Fatalf("gateway/lan4 tras portRoleTTL: role=%v, esperaba hoja (%v)", got, roleLeaf)
	}
	if got := fg.roles.role("gateway", "lan2"); got != roleDown {
		t.Fatalf("gateway/lan2: role=%v, esperaba de bajada (%v)", got, roleDown)
	}
	if d := mustDevice(t, devs, fgMacPi.String()); d.RouterID != "gateway" {
		t.Fatalf("PC en la lan4 del gateway: RouterID=%q, esperaba gateway", d.RouterID)
	}
}

// TestFleetGraphDownstreamRoleSurvivesGap: un AP que sigue en su sitio conserva
// su puerto de bajada en los sondeos donde su MAC falta en el FDB del gateway,
// hasta que el hueco supera portRoleTTL.
func TestFleetGraphDownstreamRoleSurvivesGap(t *testing.T) {
	polled1 := recabledAPPolled("lan4", true)
	l := NewLive(nil, nil, routersFromPolled(polled1), nil)
	t0 := int64(1_700_000_000_000)
	l.buildFleetGraph(polled1, nil, nil, t0)

	gap := recabledAPPolled("lan4", true)
	delete(gap["gateway"].fdb, fgMacBase)
	delete(gap["gateway"].fdb, fgMacPi)
	for _, c := range []struct {
		at   int64
		want portRole
	}{
		{t0 + portRoleTTL.Milliseconds() - 60_000, roleDown},
		{t0 + portRoleTTL.Milliseconds() + 60_000, roleLeaf},
	} {
		fg := l.buildFleetGraph(gap, nil, nil, c.at)
		if got := fg.roles.role("gateway", "lan4"); got != c.want {
			t.Errorf("gateway/lan4 %d ms tras la última evidencia: role=%v, esperaba %v", c.at-t0, got, c.want)
		}
	}
}

// uplinkDepths solo da profundidad a las unidades cuya cadena de uplinks
// llega al gateway; buildFleetGraph mantiene su respaldo para el resto.
func TestUplinkDepthsAnchoredOnly(t *testing.T) {
	ids := []string{"gw", "sw1", "sw2", "swX", "a", "b", "c"}
	parent := map[string]topoParent{
		"sw1": {parent: "gw"},
		"sw2": {parent: "sw1"},
		"c":   {parent: "swX"}, // cadena cortada: swX no tiene uplink
		"a":   {parent: "b"},   // ciclo a <-> b
		"b":   {parent: "a"},
	}
	got := uplinkDepths("gw", ids, parent)
	want := map[string]int{"gw": 0, "sw1": 1, "sw2": 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uplinkDepths = %v, esperaba %v", got, want)
	}
	for u, w := range map[string]int{"sw2": 2, "swX": 1, "c": 2, "a": len(parent) + 1} {
		if h, _ := uplinkHops("gw", u, parent); h != w {
			t.Errorf("uplinkHops(%s) = %d, esperaba %d", u, h, w)
		}
	}
}

// Un override viejo tras recablear (base colgando de sat, cuando la física es
// gateway -> base -> sat) cierra un ciclo con el LLDP/FDB. El override baja a
// la evidencia inferida: base no toma lan3 por puerto raíz y Pi/TV siguen en
// el switch tonto de base lan3 en vez de caer al gateway.
func TestFleetGraphStaleOverrideCycle(t *testing.T) {
	polled := scenarioAPolled()
	routers := []Router{{ID: "gateway", MAC: fgMacGW}, {ID: "base", MAC: fgMacBase}, {ID: "sat", MAC: fgMacSat}}
	overrides := []TopologyOverride{{Kind: "attach", MAC: fgMacBase.String(), Parent: fgMacSat.String(), Enabled: true}}
	const now = 1_700_000_000_000
	l := NewLive(nil, nil, routersFromPolled(polled), nil)
	fg, devs, _ := l.attributeDevices(polled, routers, overrides, now)

	if got := fg.parent["base"].parent; got != "gateway" {
		t.Errorf("padre de base = %q, esperaba gateway (el override cierra un ciclo)", got)
	}
	if got := fg.parent["sat"].parent; got != "base" {
		t.Errorf("padre de sat = %q, esperaba base", got)
	}
	if fg.isRootPort("base", "lan3") || !fg.isDownPort("base", "lan3") {
		t.Errorf("base lan3 debe ser puerto de bajada, roles = %v", fg.roles["base"])
	}

	want := map[macaddr.MAC]wantDevAttr{
		fgMacPi:     {routerID: "base", port: "lan3", attachTo: "dist-base-lan3"},
		fgMacTV:     {routerID: "base", port: "lan3", attachTo: "dist-base-lan3"},
		fgMacSatCli: {routerID: "sat", port: "lan2", attachTo: ""},
	}
	for _, d := range devs {
		if w, ok := want[d.MAC]; ok {
			if d.RouterID != w.routerID || d.Port != w.port || d.AttachTo != w.attachTo {
				t.Errorf("dispositivo %s = {RouterID:%q Port:%q AttachTo:%q}, esperaba %+v",
					d.MAC, d.RouterID, d.Port, d.AttachTo, w)
			}
			delete(want, d.MAC)
		}
	}
	for mac := range want {
		t.Errorf("no se encontró el dispositivo esperado %s", mac)
	}
}

func TestMergeUplinks(t *testing.T) {
	tests := []struct {
		name                string
		fdb, lldp, override map[string]topoParent
		want                map[string]topoParent
	}{
		{
			name:     "override al mismo padre hereda port y childPort",
			fdb:      map[string]topoParent{"sat": {parent: "base", port: "lan3", childPort: "lan1"}},
			override: map[string]topoParent{"sat": {parent: "base"}},
			want:     map[string]topoParent{"sat": {parent: "base", port: "lan3", childPort: "lan1"}},
		},
		{
			// El puerto lan3 es de base: no vale para gateway.
			name:     "override a otro padre no hereda el puerto",
			fdb:      map[string]topoParent{"sat": {parent: "base", port: "lan3", childPort: "lan1"}},
			override: map[string]topoParent{"sat": {parent: "gw"}},
			want:     map[string]topoParent{"sat": {parent: "gw"}},
		},
		{
			name:     "override que cierra un ciclo baja al LLDP",
			fdb:      map[string]topoParent{"base": {parent: "gw", port: "lan2"}, "sat": {parent: "base", port: "lan3"}},
			lldp:     map[string]topoParent{"base": {parent: "gw", port: "lan2", childPort: "lan1"}},
			override: map[string]topoParent{"base": {parent: "sat"}},
			want: map[string]topoParent{
				"base": {parent: "gw", port: "lan2", childPort: "lan1"},
				"sat":  {parent: "base", port: "lan3"},
			},
		},
		{
			name:     "dos overrides cruzados sin más evidencia se quedan sin uplink",
			fdb:      map[string]topoParent{"sw1": {parent: "gw", port: "lan1"}},
			override: map[string]topoParent{"a": {parent: "b"}, "b": {parent: "a"}},
			want:     map[string]topoParent{"sw1": {parent: "gw", port: "lan1"}},
		},
		{
			name:     "la cola que entra en un ciclo conserva su uplink",
			override: map[string]topoParent{"a": {parent: "b"}, "b": {parent: "a"}, "c": {parent: "a"}},
			want:     map[string]topoParent{"c": {parent: "a"}},
		},
		{
			name: "descarta gateway, padre vacío y autoenlace",
			fdb:  map[string]topoParent{"gw": {parent: "sw1"}, "x": {parent: ""}, "y": {parent: "y"}},
			want: map[string]topoParent{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeUplinks("gw", tt.fdb, tt.lldp, tt.override)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeUplinks = %+v, esperaba %+v", got, tt.want)
			}
		})
	}
}

// #2154 con el grafo: el desempate entre hojas es por routerID, así que el
// test de upstream pasa por orden alfabético. Aquí la unidad con el puerto
// bat* va primero y aun así el cliente queda en la unidad de la boca LAN física.
func TestResolveWiredFdbSkipsBatadvBackhaul(t *testing.T) {
	hueMAC := macaddr.MustParse("00:17:88:11:22:33")
	l := NewLive(nil, nil, nil, nil)
	polled := map[string]*routerPolled{
		"gw": {
			cfg:   RouterConfig{ID: "gw", Host: "192.168.1.1", IsGateway: true},
			brMac: macaddr.MustParse("AA:AA:AA:00:00:01"),
			arp:   map[macaddr.MAC]string{hueMAC: "192.168.40.10"},
		},
		"mx2": {
			cfg:   RouterConfig{ID: "mx2", Host: "192.168.1.2"},
			brMac: macaddr.MustParse("AA:AA:AA:00:00:02"),
			fdb:   map[macaddr.MAC]string{hueMAC: "lan3"},
		},
		"mx1": {
			cfg:   RouterConfig{ID: "mx1", Host: "192.168.1.3"},
			brMac: macaddr.MustParse("AA:AA:AA:00:00:03"),
			fdb:   map[macaddr.MAC]string{hueMAC: "bat0.40"},
		},
	}
	l.updateFdbMemo(polled, 1_000_000)
	for _, d := range l.buildDevices(polled, "gw") {
		if d.MAC == hueMAC && d.RouterID != "mx2" {
			t.Fatalf("Hue Bridge en %q, esperaba mx2", d.RouterID)
		}
	}
}
