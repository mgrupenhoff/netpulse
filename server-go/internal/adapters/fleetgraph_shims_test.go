package adapters

import (
	"time"

	"github.com/gnacho/netpulse/agent/macaddr"
)

// Shims con las firmas previas al grafo de la flota, para que los tests
// existentes sigan llamando al pipeline de un tick sin construir el grafo. El
// código de producción construye el grafo una vez por tick y llama a las
// variantes *WithGraph.

func (l *Live) buildDevices(polled map[string]*routerPolled, wanPort string) []Device {
	return l.buildDevicesWithGraph(polled, l.buildFleetGraph(polled, nil, nil, time.Now().UnixMilli()), wanPort)
}

func (l *Live) buildDevicesWan(polled map[string]*routerPolled, gwID, wanPort string) ([]Device, map[macaddr.MAC]bool) {
	return l.buildDevicesWithGraphWan(polled, l.buildFleetGraph(polled, nil, nil, time.Now().UnixMilli()), gwID, wanPort)
}

func (l *Live) updateFdbMemo(polled map[string]*routerPolled, nowMs int64) map[macaddr.MAC]fdbPortMemo {
	return l.updateFdbMemoWithGraph(polled, l.buildFleetGraph(polled, nil, nil, nowMs), nowMs)
}

// inferTopology corre con un grafo vacío, que conserva el comportamiento
// previo al grafo que fijan los tests de topología existentes.
func inferTopology(polled map[string]*routerPolled, devices []Device) ([]Device, []DistributionNode) {
	return inferTopologyWithGraph(polled, devices, &fleetGraph{})
}
