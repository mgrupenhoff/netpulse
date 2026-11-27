package adapters

import (
	"testing"

	"github.com/gnacho/netpulse/agent/probe"
	"github.com/gnacho/netpulse/server-go/internal/db"
)

// #2183: un vecino del segmento WAN que una versión anterior guardó en el
// registro (device_seen) no puede volver como fantasma offline. Con IP de
// puerta de enlace WAN, ni el vecino (wanIgnored) ni el peer de la ruta por
// defecto (#1791) llegan a ser dispositivos, así que la purga no puede
// depender de que estén en la lista con WanSide.
func TestWanSegmentNeighbourLeavesNoGhost(t *testing.T) {
	const (
		peer      = "0C:EF:15:00:00:01" // peer de la ruta por defecto
		neighbour = "0C:EF:15:00:00:02" // otro abonado del segmento
		lanMAC    = "AA:BB:CC:00:00:01"
		wifiMAC   = "AA:BB:CC:00:00:02" // cliente Wi-Fi con entrada ARP en la boca WAN
	)

	cases := []struct {
		name       string
		wanPort    string
		peerDev    string
		gatewayIP  string   // wanInfo.Gateway
		peerIP     string   // IP del peer en el ARP
		neighbours []string // MACs del segmento que se deben purgar
	}{
		// El caso del issue: DHCP sobre eth1, vecinos y peer en esa boca.
		{name: "vecinos en la boca WAN", wanPort: "eth1", peerDev: "eth1", gatewayIP: "203.0.113.1", peerIP: "203.0.113.1", neighbours: []string{peer, neighbour}},
		// PPPoE: la boca WAN es pppoe-wan y el peer no está en ella. Lo
		// absorbe #1791 por su IP, y también se purga.
		{name: "peer fuera de la boca WAN (PPPoE)", wanPort: "pppoe-wan", peerDev: "eth1", gatewayIP: "203.0.113.1", peerIP: "203.0.113.1", neighbours: []string{peer}},
		// La IP del peer se compara sin distinguir mayúsculas, como en el
		// bloque #1485: una IPv6 puede llegar escrita de las dos formas.
		{name: "peer IPv6 con otra capitalización", wanPort: "pppoe-wan", peerDev: "eth1", gatewayIP: "2001:DB8::1", peerIP: "2001:db8::1", neighbours: []string{peer}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := db.Open(t.TempDir())
			if err != nil {
				t.Fatalf("db.Open: %v", err)
			}
			defer d.Close()

			// Filas que dejaron versiones anteriores (first_seen=1): los
			// vecinos del segmento, el peer (anterior a #1791) y dos clientes
			// de la LAN que no se pueden tocar.
			for _, mac := range []string{peer, neighbour, lanMAC, wifiMAC} {
				if _, err := d.Exec(`INSERT INTO device_seen (mac, first_seen, last_seen, name) VALUES (?, 1, 1, '')`, mac); err != nil {
					t.Fatalf("sembrar %s: %v", mac, err)
				}
			}

			l := NewLive(nil, d, nil, nil)
			l.gatewayCfg = &RouterConfig{ID: "gw", Name: "Gateway", IsGateway: true}
			polled := map[string]*routerPolled{
				"gw": {
					cfg:     RouterConfig{ID: "gw", Name: "Gateway", IsGateway: true},
					wanInfo: probe.WanInfo{Gateway: tc.gatewayIP, Port: tc.wanPort},
					arp: map[string]string{
						peer:      tc.peerIP,
						neighbour: "203.0.113.248",
						lanMAC:    "192.168.1.60",
						wifiMAC:   "192.168.1.61",
					},
					arpDevs: map[string]string{
						peer:      tc.peerDev,
						neighbour: "eth1",
						lanMAC:    "br-lan",
						wifiMAC:   tc.wanPort,
					},
					wireless: map[string]WirelessClient{wifiMAC: {SignalDbm: -55, Band: "5 GHz"}},
				},
			}

			// El mismo orden que buildOverview: dispositivos, purga, registro.
			devices, wanSegment := l.buildDevicesWan(polled, "gw", tc.wanPort)
			for _, mac := range []string{lanMAC, wifiMAC} {
				if wanSegment[mac] {
					t.Errorf("%s es un cliente de la LAN y no puede estar en wanSegment: %v", mac, wanSegment)
				}
			}
			l.purgeWanSideSeen(devices, wanSegment)
			l.noteDevicesSeen(devices, 2000)

			for _, mac := range tc.neighbours {
				if n := countRows(t, d, "SELECT COUNT(*) FROM device_seen WHERE mac = ?", mac); n != 0 {
					t.Errorf("%s sigue en device_seen (%d filas)", mac, n)
				}
			}
			ghosts := map[string]bool{}
			for _, g := range l.ghostDevices(devices) {
				ghosts[g.MAC] = true
			}
			for _, mac := range tc.neighbours {
				if ghosts[mac] {
					t.Errorf("%s vuelve como fantasma offline", mac)
				}
			}
			// Gate negativo: las filas de los clientes de la LAN son las de
			// antes. noteDevicesSeen las recrearía con first_seen=2000 si la
			// purga las hubiera borrado, así que se mira first_seen, no si
			// existen.
			for _, mac := range []string{lanMAC, wifiMAC} {
				if n := countRows(t, d, "SELECT COUNT(*) FROM device_seen WHERE mac = ? AND first_seen = 1", mac); n != 1 {
					t.Errorf("%s es un cliente de la LAN y su fila no se toca (filas con first_seen=1: %d)", mac, n)
				}
			}
		})
	}
}
