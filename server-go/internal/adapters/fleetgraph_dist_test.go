package adapters

import (
	"testing"

	"github.com/gnacho/netpulse/agent/macaddr"
	"github.com/gnacho/netpulse/server-go/internal/db"
)

// Fixture de una red real (MACs y nombres anonimizados): gw (gateway, única
// boca LAN eth1) cableado directo a base (br0 y eth0 propia en selfMacs),
// base/lan3 hacia un switch no gestionado (2 clientes + office), office/lan1
// con 8 clientes, base/ath2 hacia garage (backhaul wifi) y ap2 sin boca en gw.
// Ninguna MAC de eth1 es cliente propio de gw: no debe inferirse switch ahí.
var (
	snapMacGw       = macaddr.MustParse("02:00:00:00:00:01")
	snapMacBase     = macaddr.MustParse("02:00:00:00:00:10")
	snapMacBaseEth0 = macaddr.MustParse("02:00:00:00:00:11")
	snapMacOffice   = macaddr.MustParse("02:00:00:00:00:20")
	snapMacGarage   = macaddr.MustParse("02:00:00:00:00:30")
	snapMacAp2      = macaddr.MustParse("02:00:00:00:00:40")
	snapMacAlias    = macaddr.MustParse("02:00:00:00:01:99")
	snapMacWatch    = macaddr.MustParse("02:00:00:00:01:98")
)

// Las demás MACs del FDB de gw/eth1, atribuidas a base, office o garage.
var (
	snapBaseLan3Clients = []macaddr.MAC{
		macaddr.MustParse("02:00:00:00:01:01"),
		macaddr.MustParse("02:00:00:00:01:02"),
	}
	snapOfficeLan1Clients = []macaddr.MAC{
		macaddr.MustParse("02:00:00:00:01:03"),
		macaddr.MustParse("02:00:00:00:01:04"),
		macaddr.MustParse("02:00:00:00:01:05"),
		macaddr.MustParse("02:00:00:00:01:06"),
		macaddr.MustParse("02:00:00:00:01:07"),
		macaddr.MustParse("02:00:00:00:01:08"),
		macaddr.MustParse("02:00:00:00:01:09"),
		macaddr.MustParse("02:00:00:00:01:0A"),
	}
	snapBaseWifiClients = []macaddr.MAC{
		macaddr.MustParse("02:00:00:00:01:0B"),
		macaddr.MustParse("02:00:00:00:01:0C"),
		macaddr.MustParse("02:00:00:00:01:0D"),
		macaddr.MustParse("02:00:00:00:01:0E"),
		macaddr.MustParse("02:00:00:00:01:0F"),
		macaddr.MustParse("02:00:00:00:01:10"),
		macaddr.MustParse("02:00:00:00:01:11"),
		macaddr.MustParse("02:00:00:00:01:12"),
		macaddr.MustParse("02:00:00:00:01:13"),
	}
	snapOfficeWifiClients = []macaddr.MAC{
		macaddr.MustParse("02:00:00:00:01:14"),
		macaddr.MustParse("02:00:00:00:01:15"),
		macaddr.MustParse("02:00:00:00:01:16"),
		macaddr.MustParse("02:00:00:00:01:17"),
		macaddr.MustParse("02:00:00:00:01:18"),
		macaddr.MustParse("02:00:00:00:01:19"),
		macaddr.MustParse("02:00:00:00:01:1A"),
		macaddr.MustParse("02:00:00:00:01:1B"),
		macaddr.MustParse("02:00:00:00:01:1C"),
	}
	snapGarageWifiClients = []macaddr.MAC{
		macaddr.MustParse("02:00:00:00:01:1D"),
		macaddr.MustParse("02:00:00:00:01:1E"),
		macaddr.MustParse("02:00:00:00:01:1F"),
		macaddr.MustParse("02:00:00:00:01:20"),
		macaddr.MustParse("02:00:00:00:01:21"),
		macaddr.MustParse("02:00:00:00:01:22"),
		macaddr.MustParse("02:00:00:00:01:23"),
	}
)

func gwSnapshotPolled(aliasOnOfficeWifi bool) map[string]*routerPolled {
	gwFdb := map[macaddr.MAC]string{
		snapMacBase:     "eth1",
		snapMacBaseEth0: "eth1",
		snapMacOffice:   "eth1",
		snapMacGarage:   "eth1",
		snapMacAlias:    "eth1",
		snapMacWatch:    "eth1",
	}
	baseFdb := map[macaddr.MAC]string{
		snapMacOffice: "lan3",
		snapMacGarage: "ath2",
	}
	officeFdb := map[macaddr.MAC]string{}
	baseWifi := map[macaddr.MAC]WirelessClient{}
	officeWifi := map[macaddr.MAC]WirelessClient{}
	garageWifi := map[macaddr.MAC]WirelessClient{}

	for _, mac := range snapBaseLan3Clients {
		gwFdb[mac] = "eth1"
		baseFdb[mac] = "lan3"
	}
	for _, mac := range snapOfficeLan1Clients {
		gwFdb[mac] = "eth1"
		officeFdb[mac] = "lan1"
	}
	for _, mac := range snapBaseWifiClients {
		gwFdb[mac] = "eth1"
		baseWifi[mac] = WirelessClient{Band: "5 GHz", SignalDbm: -55}
	}
	for _, mac := range snapOfficeWifiClients {
		gwFdb[mac] = "eth1"
		officeWifi[mac] = WirelessClient{Band: "2.4 GHz", SignalDbm: -60}
	}
	for _, mac := range snapGarageWifiClients {
		gwFdb[mac] = "eth1"
		garageWifi[mac] = WirelessClient{Band: "5 GHz", SignalDbm: -62}
	}
	if aliasOnOfficeWifi {
		officeWifi[snapMacAlias] = WirelessClient{Band: "5 GHz", SignalDbm: -43}
	}

	return map[string]*routerPolled{
		"gw": {
			cfg:    RouterConfig{ID: "gw", Name: "gw", Type: "openwrt", IsGateway: true},
			brMac:  snapMacGw,
			fdb:    gwFdb,
			arp:    map[macaddr.MAC]string{snapMacAlias: "192.168.0.119"},
			leases: []DhcpLease{{MAC: snapMacAlias, IP: "192.168.0.119"}},
		},
		"base": {
			cfg:       RouterConfig{ID: "base", Name: "base", Type: "external", AgentOnly: true},
			agentKind: "external",
			brMac:     snapMacBase,
			selfMacs:  []macaddr.MAC{snapMacBase, snapMacBaseEth0},
			fdb:       baseFdb,
			wireless:  baseWifi,
		},
		"office": {
			cfg:       RouterConfig{ID: "office", Name: "office", Type: "external", AgentOnly: true},
			agentKind: "external",
			brMac:     snapMacOffice,
			selfMacs:  []macaddr.MAC{snapMacOffice},
			fdb:       officeFdb,
			wireless:  officeWifi,
		},
		"garage": {
			cfg:       RouterConfig{ID: "garage", Name: "garage", Type: "external", AgentOnly: true},
			agentKind: "external",
			brMac:     snapMacGarage,
			selfMacs:  []macaddr.MAC{snapMacGarage},
			fdb:       map[macaddr.MAC]string{},
			wireless:  garageWifi,
		},
		"ap2": {
			cfg:   RouterConfig{ID: "ap2", Name: "ap2", Type: "openwrt"},
			brMac: snapMacAp2,
			fdb:   map[macaddr.MAC]string{},
		},
	}
}

func TestFleetGraphDownPortNoEmptySwitch(t *testing.T) {
	tests := []struct {
		name              string
		aliasOnOfficeWifi bool
		linkAliasTo       string
		aliasSettled      bool
	}{
		{
			name:              "MAC enlazada como alias en device_links",
			aliasOnOfficeWifi: true,
			linkAliasTo:       "02:00:00:00:01:09", // cliente cableado de office
			aliasSettled:      true,
		},
		{
			name:              "MAC sin enlazar retenida por settleTransient",
			aliasOnOfficeWifi: false,
			aliasSettled:      false,
		},
		{
			name:              "MAC sin enlazar como cliente cableado real junto a la selfMac de base",
			aliasOnOfficeWifi: false,
			aliasSettled:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := openLiveTestDB(t)
			now := int64(1_700_000_000_000)

			polled := gwSnapshotPolled(tt.aliasOnOfficeWifi)
			l := NewLive(nil, d, routersFromPolled(polled), nil)
			l.mu.Lock()
			l.lastPolled = polled
			l.mu.Unlock()

			// Watch offline atribuido previamente por wifi a base (sembrado tras
			// NewLive porque la migración attrib_v2 vacía device_attrib al abrir).
			if _, err := d.Exec(
				`INSERT INTO device_attrib (mac, router_id, band, signal_dbm, last_seen) VALUES (?, ?, ?, ?, ?)`,
				snapMacWatch.String(), "base", "2.4 GHz", -70, db.NowMS(),
			); err != nil {
				t.Fatalf("sembrar device_attrib: %v", err)
			}

			// Registro previo en device_seen (incluyendo eth0 de base de antes de selfMacs).
			seedSeen := []macaddr.MAC{snapMacBaseEth0}
			seedSeen = append(seedSeen, snapBaseLan3Clients...)
			seedSeen = append(seedSeen, snapOfficeLan1Clients...)
			seedSeen = append(seedSeen, snapBaseWifiClients...)
			seedSeen = append(seedSeen, snapOfficeWifiClients...)
			seedSeen = append(seedSeen, snapGarageWifiClients...)
			if tt.aliasSettled {
				seedSeen = append(seedSeen, snapMacAlias)
			}
			for _, mac := range seedSeen {
				if _, err := d.Exec(
					`INSERT INTO device_seen (mac, first_seen, last_seen) VALUES (?, ?, ?)`,
					mac.String(), now-3_600_000, now-60_000,
				); err != nil {
					t.Fatalf("sembrar device_seen %s: %v", mac, err)
				}
			}

			if tt.linkAliasTo != "" {
				if _, err := d.Exec(
					`INSERT INTO device_links (mac, canonical) VALUES (?, ?)`,
					snapMacAlias.String(), tt.linkAliasTo,
				); err != nil {
					t.Fatalf("sembrar device_links: %v", err)
				}
			}

			// Pipeline de buildOverview: grafo -> memo -> devices -> topología -> filtros -> registro -> fantasmas.
			_, devs, dists := l.attributeDevices(polled, nil, nil, now)
			devs = l.filterTombstoned(devs)
			devs = l.settleTransient(devs, now)
			l.noteDevicesSeen(devs, now)
			devs = append(devs, l.ghostDevices(devs)...)

			distByID := map[string]DistributionNode{}
			for _, dn := range dists {
				distByID[dn.ID] = dn
			}
			if dn, bad := distByID["dist-gw-eth1"]; bad {
				attached := []string{}
				for _, dev := range devs {
					if dev.AttachTo == dn.ID {
						attached = append(attached, dev.MAC.String())
					}
				}
				t.Errorf("no debía crearse dist-gw-eth1, obtuve %+v (adjuntos=%v)", dn, attached)
			}
			for id, wantCount := range map[string]int{
				"dist-base-lan3":   2,
				"dist-office-lan1": 8,
			} {
				dn, ok := distByID[id]
				if !ok {
					t.Errorf("faltaba el distnode esperado %s en %+v", id, dists)
					continue
				}
				if dn.Kind != "inferred" || dn.MacCount != wantCount {
					t.Errorf("distnode %s = %+v, esperaba Kind=inferred MacCount=%d", id, dn, wantCount)
				}
			}
		})
	}
}

// TestFleetGraphNilDescendantNoEmptyDist verifica que bloquear los residuos en
// puertos interiores de bajada cuando un descendiente llega con fdb == nil no
// deje switches inferidos vacíos en los padres.
func TestFleetGraphNilDescendantNoEmptyDist(t *testing.T) {
	polled := scenarioAPolled()
	polled["sat"].fdb = nil
	l := NewLive(nil, nil, routersFromPolled(polled), nil)
	_, dists := runFleetGraphTick(l, polled, 1_700_000_000_000)
	if len(dists) != 0 {
		t.Fatalf("esperaba 0 distnodes cuando sat tiene fdb nil, obtuve %+v", dists)
	}
}
