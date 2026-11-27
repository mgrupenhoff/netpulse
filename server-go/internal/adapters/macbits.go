// macbits.go — clasificación de una MAC por sus bits de control.
package adapters

import "github.com/gnacho/netpulse/agent/macaddr"

// persistLocalMAC decide si una MAC debe recordarse en device_attrib (#1818).
// Una dirección localmente administrada sin IP ni lease es una identidad
// transitoria: el NMRP que anuncia un Netgear mientras reinicia para flashear
// vive segundos y no pide dirección. Recordarla es lo que la mantenía en la
// lista de clientes después de desaparecer. Todo lo demás se persiste igual.
func persistLocalMAC(mac macaddr.MAC, ip string, hasLease, wasKnown bool) bool {
	if !mac.IsLocallyAdministered() {
		return true
	}
	// Una MAC que ya estaba en el inventario se sigue actualizando: el filtro
	// solo evita estrenar una identidad transitoria.
	return ip != "" || hasLease || wasKnown
}
