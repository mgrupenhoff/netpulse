package adapters

import (
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

// fleetGraph: árbol dirigido de la flota y clasificación de puertos (raíz de
// subida, interior de bajada, hoja) de un tick de sondeo. Los llamadores
// siempre pasan un grafo no nil. Un &fleetGraph{} vacío no clasifica ningún
// puerto, así que toda consulta de puerto sobre él devuelve false.
type fleetGraph struct {
	gatewayID  string
	ownerOf    map[string]string     // upper(MAC) -> routerID de toda MAC de identidad de la flota
	lldpEv     map[string]topoParent // childID -> enlace al padre según LLDP
	fdbEv      map[string]topoParent // childID -> enlace al padre según FDB
	overrideEv map[string]topoParent // childID -> enlace al padre según overrides manuales
	parent     map[string]topoParent // childID -> uplink elegido (override > lldp > fdb)
	depth      map[string]int        // routerID -> saltos desde el gateway (gateway = 0)
	roles      portRoles             // routerID -> puerto -> rol (raíz / interior de bajada)
}

// portRole: hacia dónde lleva un puerto del router dentro de la flota.
type portRole uint8

const (
	roleLeaf portRole = iota // dispositivos finales, quizá tras un switch (valor cero)
	roleRoot                 // subida: lleva hacia el gateway
	roleDown                 // interior de bajada: lleva a una unidad descendiente
)

// portRoles: routerID -> puerto -> rol. Cada puerto tiene un solo rol, así que
// no puede ser de subida y de bajada a la vez. Los puertos ausentes son hojas.
type portRoles map[string]map[string]portRole

func (pr portRoles) role(routerID, port string) portRole {
	return pr[routerID][port]
}

// hasRoot indica si se conoce el puerto de uplink de routerID.
func (pr portRoles) hasRoot(routerID string) bool {
	for _, r := range pr[routerID] {
		if r == roleRoot {
			return true
		}
	}
	return false
}

// portRoleTTL acota cuánto sobrevive un rol recordado a su última evidencia
// fresca. El propio tráfico de una unidad cableada lo renueva en casi cada
// sondeo, así que diez minutos cubren muchos sondeos perdidos y el doble del
// tiempo de caducidad del FDB. Caduca cuando la unidad desaparece o se
// recablea: el puerto de bajada obsoleto deja entonces de tratar a un cliente
// enchufado en él como tránsito de uplink. Es más corto que fdbStickyTTL
// porque, hasta que caduca, el paso 5 de buildDevicesWithGraph mantiene a ese
// cliente en la unidad que colgaba de ese puerto.
const portRoleTTL = 10 * time.Minute

type rememberedRole struct {
	role   portRole
	seenMs int64 // último tick con evidencia fresca de este rol (epoch ms)
}

// portRoleMemo (#694) conserva los roles de puerto entre ticks, mientras las
// entradas FDB de las propias unidades caducan entre uno y otro.
type portRoleMemo map[routerPort]rememberedRole

// update aplica la evidencia fresca de un router. Los puertos raíz frescos
// sustituyen a los recordados, así un uplink recableado se mueve. La evidencia
// de bajada nunca degrada un puerto raíz.
func (m portRoleMemo) update(routerID string, freshRoot, freshDown map[string]bool, nowMs int64) {
	if len(freshRoot) > 0 {
		for k, r := range m {
			if k.router == routerID && r.role == roleRoot && !freshRoot[k.port] {
				delete(m, k)
			}
		}
		for port := range freshRoot {
			m[routerPort{routerID, port}] = rememberedRole{roleRoot, nowMs}
		}
	}
	for port := range freshDown {
		k := routerPort{routerID, port}
		if m[k].role != roleRoot {
			m[k] = rememberedRole{roleDown, nowMs}
		}
	}
}

// expire olvida los roles sin evidencia fresca durante portRoleTTL, incluidos
// los de routers que ya no están en la flota.
func (m portRoleMemo) expire(nowMs int64) {
	ttlMs := portRoleTTL.Milliseconds()
	for k, r := range m {
		if nowMs-r.seenMs > ttlMs {
			delete(m, k)
		}
	}
}

// roles devuelve una copia de los roles recordados para el grafo de un tick.
func (m portRoleMemo) roles() portRoles {
	out := portRoles{}
	for k, r := range m {
		if out[k.router] == nil {
			out[k.router] = map[string]portRole{}
		}
		out[k.router][k.port] = r.role
	}
	return out
}

func (fg *fleetGraph) isRootPort(routerID, port string) bool {
	return fg.roles.role(routerID, port) == roleRoot
}

func (fg *fleetGraph) isDownPort(routerID, port string) bool {
	return fg.roles.role(routerID, port) == roleDown
}

func (fg *fleetGraph) isInteriorPort(routerID, port string) bool {
	return fg.roles.role(routerID, port) != roleLeaf
}

// canInferSwitch indica si se sabe que el puerto de una unidad no gateway no
// es su uplink (o está clasificado como de bajada, o se conoce el puerto raíz
// de la unidad y ya se ha saltado).
func (fg *fleetGraph) canInferSwitch(routerID, port string) bool {
	return fg.roles.role(routerID, port) == roleDown || fg.roles.hasRoot(routerID)
}

func isAncestor(parent map[string]topoParent, gatewayID, cand, rid string) bool {
	if cand == "" || rid == "" || cand == rid {
		return false
	}
	if cand == gatewayID {
		return true
	}
	cur := rid
	for i := 0; i <= len(parent); i++ {
		p, ok := parent[cur]
		if !ok || p.parent == "" {
			return false
		}
		if p.parent == cand {
			return true
		}
		cur = p.parent
	}
	return false
}

// mergeUplinks elige el uplink de cada unidad por precedencia: override >
// lldp > fdb (la misma de BuildTopoSemantics). Descarta el gateway, los
// padres vacíos y los autoenlaces. Un override sin puerto hereda port y
// childPort de la evidencia que pisa solo si apunta al mismo padre: con otro
// padre ese puerto es de otra unidad.
//
// Cada fuente por separado no tiene ciclos, pero la mezcla sí puede tenerlos
// (p. ej. un override viejo tras recablear contradice al LLDP y a la FDB). En
// cada ciclo, las unidades cuyo uplink viene de la fuente más fuerte del
// ciclo bajan a su siguiente fuente; sin más fuentes se quedan sin uplink.
func mergeUplinks(gwID string, fdbEv, lldpEv, overrideEv map[string]topoParent) map[string]topoParent {
	sources := []map[string]topoParent{fdbEv, lldpEv, overrideEv} // de menor a mayor precedencia
	const overrideLvl = 2
	valid := func(rid string, ev topoParent) bool {
		return rid != gwID && ev.parent != "" && ev.parent != rid
	}
	// pick: el uplink de rid desde la fuente lvl o, si no lo tiene, desde la
	// más fuerte por debajo. Devuelve también el nivel elegido.
	var pick func(rid string, lvl int) (topoParent, int, bool)
	pick = func(rid string, lvl int) (topoParent, int, bool) {
		for ; lvl >= 0; lvl-- {
			ev, ok := sources[lvl][rid]
			if !ok || !valid(rid, ev) {
				continue
			}
			if lvl != overrideLvl {
				return ev, lvl, true // LLDP pisa la FDB entera (#1279)
			}
			if old, _, ok := pick(rid, lvl-1); ok && old.parent == ev.parent {
				if ev.port == "" {
					ev.port = old.port
				}
				if ev.childPort == "" {
					ev.childPort = old.childPort
				}
			}
			return ev, lvl, true
		}
		return topoParent{}, -1, false
	}

	ids := map[string]bool{}
	for _, src := range sources {
		for rid := range src {
			ids[rid] = true
		}
	}
	sorted := slices.Sorted(maps.Keys(ids))
	parent := map[string]topoParent{}
	level := map[string]int{}
	for _, rid := range sorted {
		if ev, lvl, ok := pick(rid, len(sources)-1); ok {
			parent[rid] = ev
			level[rid] = lvl
		}
	}

	// Cada pasada baja de nivel o quita al menos un uplink, así que acaba.
	for pass := 0; pass <= len(sources)*len(parent); pass++ {
		var demote []string
		for _, rid := range sorted {
			cyc := uplinkCycle(parent, rid)
			if cyc == nil {
				continue
			}
			top := -1
			for _, m := range cyc {
				top = max(top, level[m])
			}
			if level[rid] == top {
				demote = append(demote, rid)
			}
		}
		if len(demote) == 0 {
			break
		}
		for _, rid := range demote {
			if ev, lvl, ok := pick(rid, level[rid]-1); ok {
				parent[rid] = ev
				level[rid] = lvl
			} else {
				delete(parent, rid)
				delete(level, rid)
			}
		}
	}
	return parent
}

// uplinkCycle devuelve las unidades del ciclo de uplinks por el que pasa rid,
// o nil si subiendo desde rid no se vuelve a rid.
func uplinkCycle(parent map[string]topoParent, rid string) []string {
	cyc := []string{rid}
	cur := rid
	for step := 0; step <= len(parent); step++ {
		p, ok := parent[cur]
		if !ok {
			return nil
		}
		if p.parent == rid {
			return cyc
		}
		cur = p.parent
		cyc = append(cyc, cur)
	}
	return nil
}

// uplinkHops cuenta los saltos de u hasta el gateway subiendo por parent.
// anchored dice si la cadena llega al gateway. Si se corta antes, la unidad
// de arriba cuenta un salto más, como si colgara del gateway. El tope de
// pasos corta un ciclo.
func uplinkHops(gwID, u string, parent map[string]topoParent) (hops int, anchored bool) {
	cur := u
	for step := 0; step <= len(parent); step++ {
		p, ok := parent[cur]
		if !ok || p.parent == "" {
			if cur != gwID {
				hops++
			}
			return hops, false
		}
		hops++
		cur = p.parent
		if cur == gwID {
			return hops, true
		}
	}
	return hops, false
}

// uplinkDepths: saltos hasta el gateway (gateway = 0) de las unidades de ids
// cuya cadena de uplinks llega a él. Las que no llegan quedan fuera del mapa.
func uplinkDepths(gwID string, ids []string, parent map[string]topoParent) map[string]int {
	depth := map[string]int{}
	if gwID != "" {
		depth[gwID] = 0
	}
	for _, u := range ids {
		if u == gwID {
			continue
		}
		if hops, anchored := uplinkHops(gwID, u, parent); anchored {
			depth[u] = hops
		}
	}
	return depth
}

// buildFleetGraph construye el árbol dirigido de la flota y clasifica los
// puertos del bridge de cada unidad en raíz (subida) e interiores de bajada,
// combinando las observaciones frescas con la memo persistente de roles. nowMs
// fecha la evidencia fresca y caduca los roles obsoletos.
func (l *Live) buildFleetGraph(polled map[string]*routerPolled, routerList []Router, overrides []TopologyOverride, nowMs int64) *fleetGraph {
	ids := make([]string, 0, len(polled))
	for id, p := range polled {
		if p != nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	gwID := ""
	l.mu.Lock()
	if l.gatewayCfg != nil {
		gwID = l.gatewayCfg.ID
	}
	l.mu.Unlock()
	if gwID == "" {
		for _, id := range ids {
			if polled[id].cfg.IsGateway {
				gwID = id
				break
			}
		}
	}

	if len(routerList) == 0 {
		routerList = make([]Router, 0, len(ids))
		for _, id := range ids {
			routerList = append(routerList, Router{ID: id, MAC: polled[id].brMac})
		}
	}
	if overrides == nil && l.db != nil {
		overrides = loadTopologyOverrides(l.db)
	}

	// #1279: uplinks resueltos por LLDP (regla de puerto raíz); mandan sobre
	// la evidencia FDB/device en BuildTopoSemantics. Se calcula ANTES que la
	// FDB para sembrar profundidades: un switch core cuya MAC de chasis nadie
	// aprende no tiene candidato padre por FDB y, sin semilla, su profundidad
	// queda desconocida y las unidades cableadas en él caen al gateway.
	lldpEv := fleetLldpEvidence(polled, gwID)
	seedDepth := map[string]int{gwID: 0}
	for pass := 0; pass <= len(lldpEv); pass++ {
		changed := false
		for child, ev := range lldpEv {
			if d, ok := seedDepth[ev.parent]; ok {
				if cur, has := seedDepth[child]; !has || cur > d+1 {
					seedDepth[child] = d + 1
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	// MACs registradas (routers.mac): permiten a fleetFdbEvidence anclar
	// unidades sin bridge MAC utilizable (DSM: docker0 en bridgeDetect, sin
	// lldpd) desde el FDB del switch que aprende su NIC (p. ej. NAS en xg4).
	registeredMacs := make(map[string]string, len(routerList))
	for _, r := range routerList {
		if r.MAC != "" {
			registeredMacs[r.ID] = r.MAC
		}
	}
	fdbEv := fleetFdbEvidence(polled, gwID, registeredMacs, seedDepth)
	// #1321: los attaches manuales (#142) cuya MAC es la bridge de una unidad
	// de flota fijan su uplink por encima de toda inferencia.
	overrideEv := fleetOverrideEvidence(overrides, routerList)

	ownerOf := map[string]string{}
	setOwner := func(mac, id string, overwrite bool) {
		if m := strings.ToUpper(strings.TrimSpace(mac)); m != "" {
			if overwrite || ownerOf[m] == "" {
				ownerOf[m] = id
			}
		}
	}
	for _, r := range routerList {
		setOwner(r.MAC, r.ID, true)
	}
	l.mu.Lock()
	for id, mac := range l.routerMacs {
		setOwner(mac, id, false)
	}
	l.mu.Unlock()
	if l.db != nil {
		if rows, err := l.db.Query("SELECT id, mac FROM routers WHERE mac IS NOT NULL AND mac != ''"); err == nil {
			for rows.Next() {
				var id, mac string
				if rows.Scan(&id, &mac) == nil {
					setOwner(mac, id, false)
				}
			}
			rows.Close()
		}
	}
	for _, id := range ids {
		p := polled[id]
		for _, mac := range p.selfMacs {
			setOwner(mac, id, true)
		}
		setOwner(p.brMac, id, true)
	}

	parent := mergeUplinks(gwID, fdbEv, lldpEv, overrideEv)
	depth := uplinkDepths(gwID, ids, parent)
	for _, u := range ids {
		if _, ok := depth[u]; !ok && u != gwID {
			// Sin camino al gateway: cuenta como si colgara de él.
			depth[u], _ = uplinkHops(gwID, u, parent)
		}
	}

	freshRoot := map[string]map[string]bool{}
	freshDown := map[string]map[string]bool{}
	for _, u := range ids {
		p := polled[u]
		if p.fdb == nil {
			continue
		}
		fr := map[string]bool{}
		if u != gwID {
			if cp := parent[u].childPort; cp != "" {
				fr[cp] = true
			}
			for mac, port := range p.fdb {
				if port != "" && isAncestor(parent, gwID, ownerOf[strings.ToUpper(mac)], u) {
					fr[port] = true
				}
			}
		}
		fd := map[string]bool{}
		for _, ev := range parent {
			if ev.parent == u && ev.port != "" && !fr[ev.port] {
				fd[ev.port] = true
			}
		}
		for mac, port := range p.fdb {
			// Solo la MAC de un descendiente hace que un puerto sea de bajada;
			// la de un hermano o primo es tránsito que llega por el uplink de u.
			if port != "" && !fr[port] && isAncestor(parent, gwID, u, ownerOf[strings.ToUpper(mac)]) {
				fd[port] = true
			}
		}
		freshRoot[u] = fr
		freshDown[u] = fd
	}

	l.mu.Lock()
	if l.portRoleMemo == nil {
		l.portRoleMemo = portRoleMemo{}
	}
	for _, u := range ids {
		l.portRoleMemo.update(u, freshRoot[u], freshDown[u], nowMs)
	}
	l.portRoleMemo.expire(nowMs)
	roles := l.portRoleMemo.roles()
	l.mu.Unlock()

	return &fleetGraph{
		gatewayID:  gwID,
		ownerOf:    ownerOf,
		lldpEv:     lldpEv,
		fdbEv:      fdbEv,
		overrideEv: overrideEv,
		parent:     parent,
		depth:      depth,
		roles:      roles,
	}
}
