// usteer_agent.go - vista usteer alimentada por el payload del agente
// (#1431). En flotas agent-only (router.AgentOnly, el patrón NetGrip con el
// agente embebido) no hay sondeo SSH y la vista de itinerancia se servía
// vacía (503) aunque usteer corriera y el agente empujara su estado completo.
// Este módulo construye el mismo shape Usteer/AP desde el payload: los APs
// LOCALES del router (los remotos ya salen como locales de su propio
// equipo), con iface resuelto por wireless.ownBssids y clientes colgados del
// AP de su banda (wireless.clients trae la banda por MAC asociada).
package adapters

import (
	"sort"

	"github.com/gnacho/netpulse/agent/macaddr"
	"github.com/gnacho/netpulse/agent/probe"
)

// usteerAPsFromPayload devuelve los APs locales de usteer del payload y
// cuántos vio (para mesh.apsSeen). (nil, 0) si el payload no trae sección
// usteer o no hay APs locales que pintar.
func usteerAPsFromPayload(name string, p *probe.Payload) ([]UsteerAP, int) {
	if p == nil || p.Data.Usteer == nil {
		return nil, 0
	}
	var ifaceByBSSID map[macaddr.MAC]string
	var clientBand map[macaddr.MAC]string
	var clientSignal map[macaddr.MAC]int
	if wd := p.Data.Wireless; wd != nil {
		ifaceByBSSID = make(map[macaddr.MAC]string, len(wd.OwnBssids))
		for _, ob := range wd.OwnBssids {
			if m, ok := macaddr.ParseOrWarn(macWarn, "usteer.own_bssid", ob.BSSID); ok {
				ifaceByBSSID[m] = ob.Iface
			}
		}
		clientBand = make(map[macaddr.MAC]string, len(wd.Clients))
		clientSignal = make(map[macaddr.MAC]int, len(wd.Clients))
		for mac, c := range wd.Clients {
			if m, ok := macaddr.ParseOrWarn(macWarn, "usteer.client_mac", mac); ok {
				clientBand[m] = c.Band
				clientSignal[m] = c.SignalDbm
			}
		}
	}

	aps := []UsteerAP{}
	for ssid, ssidData := range p.Data.Usteer.SSIDs {
		// Los agentes con el flag Local solo emiten sus APs locales: los
		// remotos del mesh ya se listan como locales de su propio router
		// (mismo criterio que el sondeo SSH, que solo lee local_info). Sin
		// flag (agentes anteriores a #417) se asume todo local.
		hasLocalFlag := false
		for _, ap := range ssidData.APs {
			if ap.Local {
				hasLocalFlag = true
				break
			}
		}
		byBand := map[string][]int{}
		for _, ap := range ssidData.APs {
			if hasLocalFlag && !ap.Local {
				continue
			}
			bssid, _ := macaddr.ParseOrWarn(macWarn, "usteer.ap_bssid", ap.BSSID)
			band := "2.4 GHz"
			if ap.Freq >= 5000 {
				band = "5 GHz"
			}
			byBand[band] = append(byBand[band], len(aps))
			aps = append(aps, UsteerAP{
				SSID:           ssid,
				BSSID:          bssid,
				Hostname:       name,
				Band:           band,
				Freq:           ap.Freq,
				UtilizationPct: ap.Load,
				ClientCount:    ap.Clients,
				Local:          true,
				Iface:          ifaceByBSSID[bssid],
			})
		}
		attachUsteerClients(aps, byBand, ssidData.Clients, clientBand, clientSignal)
	}

	// Orden determinista: los mapas del payload iteran en orden aleatorio.
	sort.Slice(aps, func(i, j int) bool {
		if aps[i].SSID != aps[j].SSID {
			return aps[i].SSID < aps[j].SSID
		}
		return aps[i].BSSID.Compare(aps[j].BSSID) < 0
	})
	return aps, len(aps)
}

// attachUsteerClients cuelga cada cliente usteer del SSID de su AP local: la
// banda de la estación (wireless.clients) decide el AP; sin dato de banda se
// usa el AP único del SSID. Señal: la de la estación si existe, si no la del
// cliente usteer; señal 0/positiva = sin datos, se ignora (mismo criterio
// que el sondeo SSH).
func attachUsteerClients(aps []UsteerAP, byBand map[string][]int, clients map[string]probe.UsteerClient, clientBand map[macaddr.MAC]string, clientSig map[macaddr.MAC]int) {
	attached := map[macaddr.MAC]bool{}
	for mac, uc := range clients {
		parsed, ok := macaddr.ParseOrWarn(macWarn, "usteer.client_mac", mac)
		if !ok || attached[parsed] {
			continue
		}
		signal := clientSig[parsed]
		if signal == 0 {
			signal = uc.Signal
		}
		if signal >= 0 {
			continue
		}
		band := clientBand[parsed]
		idxs := byBand[band]
		if len(idxs) == 0 {
			if band != "" || len(byBand) != 1 {
				continue // banda desconocida con varios APs: no se adivina
			}
			for _, v := range byBand {
				idxs = v
			}
		}
		aps[idxs[0]].Clients = append(aps[idxs[0]].Clients, UsteerClient{MAC: parsed, Signal: signal})
		attached[parsed] = true
	}
	for i := range aps {
		sort.Slice(aps[i].Clients, func(x, y int) bool { return aps[i].Clients[x].MAC.Compare(aps[i].Clients[y].MAC) < 0 })
	}
}
