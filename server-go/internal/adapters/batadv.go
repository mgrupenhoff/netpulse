package adapters

import (
	"strings"

	"github.com/gnacho/netpulse/agent/macaddr"
)

// isBatadvBlaMAC: BATMAN-adv bridge loop avoidance emite tramas LOOP DETECT
// desde una dirección localmente administrada cuyos dos primeros octetos son
// BA:BE, con los cuatro restantes aleatorios y rotados cada cierto número de
// periodos (net/batman-adv/bridge_loop_avoidance.c fija loopdetect_addr a
// 0xba 0xbe). No es un host: es una sonda del mesh. Pero el FDB de cada
// unidad la aprende, así que sin este filtro una red con batman-adv llena la
// lista de clientes de fantasmas (#1818).
func isBatadvBlaMAC(mac macaddr.MAC) bool {
	b := mac.As6()
	return b[0] == 0xBA && b[1] == 0xBE
}

// withoutBatadvBla devuelve el FDB sin las direcciones de sonda de BLA. Si no
// hay nada que quitar devuelve el MISMO mapa, así el camino normal no paga
// nada y nunca se muta el mapa del payload en sitio.
func withoutBatadvBla(fdb map[macaddr.MAC]string) map[macaddr.MAC]string {
	dropped := 0
	for mac := range fdb {
		if isBatadvBlaMAC(mac) {
			dropped++
		}
	}
	if dropped == 0 {
		return fdb
	}
	out := make(map[macaddr.MAC]string, len(fdb)-dropped)
	for mac, port := range fdb {
		if !isBatadvBlaMAC(mac) {
			out[mac] = port
		}
	}
	return out
}

// isBatadvBackhaulPort reports whether port is a BATMAN-adv mesh interface
// (e.g. bat0, bat0.40). Bridge FDB entries learned on bat* come from remote
// mesh nodes over backhaul rather than local physical LAN access ports.
func isBatadvBackhaulPort(port string) bool {
	base := strings.TrimSpace(port)
	if i := strings.IndexAny(base, ".@"); i >= 0 {
		base = base[:i]
	}
	if len(base) <= 3 || !strings.EqualFold(base[:3], "bat") {
		return false
	}
	for i := 3; i < len(base); i++ {
		if base[i] < '0' || base[i] > '9' {
			return false
		}
	}
	return true
}
