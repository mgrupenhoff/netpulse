// fdbsticky.go — memoria pegajosa del puerto FDB (issue #656).
//
// Problema: las entradas del bridge FDB caducan (~5 min) y los dispositivos
// callados (AV: TV, receptor, shield…) pierden su evidencia de puerto entre
// ticks aunque sigan online (ARP). El mapa los re-anclaba al gateway y
// "saltaban" del switch inferido donde están cableados físicamente.
//
// Solución: recordar la última boca REAL donde se vio cada MAC (fdbMemo, con
// TTL). La memo se usa como OVERLAY de inferTopology, SOLO para MACs que el
// FDB actual ya no tiene, y en el paso 5 de buildDevicesWithGraph para corregir
// la atribución de los clientes cableados que el gateway solo ve por un uplink
// (#678). La memo nunca crea presencia: si el dispositivo no está online
// por sí mismo (wireless/FDB/ARP), no se dibuja. El FDB real
// siempre manda sobre el recuerdo.
package adapters

import (
	"maps"
	"strings"
	"time"
)

// fdbStickyTTL: frescura de la memo. Los dispositivos AV despiertan y hablan
// (refrescando la memo) mucho antes; 30 min es un margen holgado que además
// acota el efecto de un dispositivo cambiado de boca.
const fdbStickyTTL = 30 * time.Minute

// fdbPortMemo: última boca real donde el FDB vio una MAC.
type fdbPortMemo struct {
	routerID string
	port     string
	ts       int64 // epoch ms
	leaf     bool  // true si se aprendió en un puerto hoja y no en un enlace interior
}

type fdbAttachment struct {
	routerID string
	port     string
	isLeaf   bool
	depth    int
}

// betterAttachment ordena los candidatos de anclaje FDB de una misma MAC de
// cliente: un puerto hoja gana a uno interior de bajada; entre interiores gana
// la unidad más profunda; entre hojas, un satélite gana al gateway.
func betterAttachment(cand, cur fdbAttachment, gatewayID string) bool {
	if cand.isLeaf != cur.isLeaf {
		return cand.isLeaf
	}
	if !cand.isLeaf && cand.depth != cur.depth {
		return cand.depth > cur.depth
	}
	if (cand.routerID == gatewayID) != (cur.routerID == gatewayID) {
		return cand.routerID != gatewayID
	}
	if cand.depth != cur.depth {
		return cand.depth > cur.depth
	}
	return cand.routerID < cur.routerID
}

// resolveWiredFdb resuelve cada MAC de cliente vista este tick en el FDB de
// alguna unidad a un único anclaje ganador (routerID, puerto) usando el árbol
// de conexiones de la flota.
func resolveWiredFdb(polled map[string]*routerPolled, fg *fleetGraph) map[string]fdbAttachment {
	out := map[string]fdbAttachment{}

	// Un descendiente con fdb == nil no puede descontar sus propios clientes
	// hoja, así que se bloquean los residuos interiores en todo puerto por
	// encima de él.
	blocked := map[string]map[string]bool{}
	for v := range fg.parent {
		if p := polled[v]; p != nil && p.fdb != nil {
			continue
		}
		for cur, step := v, 0; step <= len(fg.parent); step++ {
			ev, ok := fg.parent[cur]
			if !ok || ev.parent == "" || ev.parent == cur {
				break
			}
			if ev.port != "" {
				if blocked[ev.parent] == nil {
					blocked[ev.parent] = map[string]bool{}
				}
				blocked[ev.parent][ev.port] = true
			}
			cur = ev.parent
		}
	}

	for routerID, p := range polled {
		if p == nil {
			continue
		}
		d := fg.depth[routerID]
		for mac, port := range p.fdb {
			role := fg.roles.role(routerID, port)
			if port == "" || port == "wan" || role == roleRoot {
				continue
			}
			if fg.ownerOf[strings.ToUpper(mac)] != "" {
				continue
			}
			isLeaf := role != roleDown
			if !isLeaf && blocked[routerID][port] {
				continue
			}
			cand := fdbAttachment{
				routerID: routerID,
				port:     port,
				isLeaf:   isLeaf,
				depth:    d,
			}
			if cur, exists := out[mac]; !exists || betterAttachment(cand, cur, fg.gatewayID) {
				out[mac] = cand
			}
		}
	}
	return out
}

// updateFdbMemoWithGraph refresca la memo sticky del FDB con los anclajes
// resueltos en este tick y poda las entradas caducadas (#656, #678, #694).
func (l *Live) updateFdbMemoWithGraph(polled map[string]*routerPolled, fg *fleetGraph, nowMs int64) map[string]fdbPortMemo {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fdbMemo == nil {
		l.fdbMemo = map[string]fdbPortMemo{}
	}
	online := map[string]bool{}
	for _, p := range polled {
		if p == nil {
			continue
		}
		for mac := range p.arp {
			online[mac] = true
		}
		for _, le := range p.leases {
			if le.MAC != "" {
				online[le.MAC] = true
			}
		}
	}
	ttlMs := fdbStickyTTL.Milliseconds()
	// Una memo hoja vigente no se pisa con un residuo interior cuando un cliente
	// callado caduca en su puerto hoja antes de que el switch de arriba lo olvide.
	for mac, att := range resolveWiredFdb(polled, fg) {
		prev, exists := l.fdbMemo[mac]
		if !exists || nowMs-prev.ts > ttlMs || att.isLeaf || !prev.leaf {
			l.fdbMemo[mac] = fdbPortMemo{
				routerID: att.routerID,
				port:     att.port,
				ts:       nowMs,
				leaf:     att.isLeaf,
			}
		}
	}
	for mac, m := range l.fdbMemo {
		if nowMs-m.ts > ttlMs {
			if online[mac] {
				// #694: el device sigue online (ARP/lease) aunque no hable por
				// FDB: refresca la memo para que un cliente callado conserve su
				// boca en vez de caducar y caer al fallback del gateway.
				m.ts = nowMs
				l.fdbMemo[mac] = m
				continue
			}
			delete(l.fdbMemo, mac)
		}
	}
	// Una copia: los llamadores la leen tras soltar l.mu mientras otras
	// peticiones siguen actualizando la memo.
	return maps.Clone(l.fdbMemo)
}

// overlayStickyFdb devuelve una copia de `polled` donde cada router lleva, ADEMÁS
// de su FDB real, las MACs recordadas (memo fresca) que ya no estén presentes.
// Función pura: no toca polled ni memo. Solo la usa inferTopology.
func overlayStickyFdb(polled map[string]*routerPolled, memo map[string]fdbPortMemo, nowMs int64) map[string]*routerPolled {
	out := make(map[string]*routerPolled, len(polled))
	for id, p := range polled {
		np := *p // shallow copy: solo se sustituye fdb (map propio)
		nf := make(map[string]string, len(p.fdb)+4)
		for k, v := range p.fdb {
			nf[k] = v
		}
		np.fdb = nf
		out[id] = &np
	}
	ttlMs := fdbStickyTTL.Milliseconds()
	for mac, m := range memo {
		if nowMs-m.ts > ttlMs {
			continue
		}
		p, ok := out[m.routerID]
		if !ok {
			continue // el router ya no está en la flota
		}
		if _, exists := p.fdb[mac]; exists {
			continue // el FDB real manda
		}
		p.fdb[mac] = m.port
	}
	return out
}
