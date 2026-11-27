// net_ifaces.go — parseo de las interfaces lógicas en ejecución de OpenWrt
// (`ubus call network.interface dump`) para el descubrimiento de clientes por
// red.
package probe

import "encoding/json"

// NetIface representa una interfaz lógica de OpenWrt reportada por netifd
// (lan, wan, wan6, wwan, wg0...), con su estado y los dispositivos Linux que
// tiene asociados en este momento.
type NetIface struct {
	Name    string   `json:"name"`
	Proto   string   `json:"proto,omitempty"`
	Up      bool     `json:"up"`
	Devices []string `json:"devices,omitempty"` // l3_device, luego device; únicos y no vacíos
}

// InterfaceDump agrupa el uplink activo y las interfaces lógicas utilizables
// extraídas de una única lectura de `ubus call network.interface dump`.
type InterfaceDump struct {
	Wan    WanInfo
	Ifaces []NetIface
}

// ParseInterfaceDump decodifica una sola vez la salida de `ubus call
// network.interface dump` (`{"interface":[...]}`) y devuelve el uplink activo
// (prefiriendo preferred si viene indicado) junto con las interfaces lógicas
// utilizables (sin dinámicas ni loopback, manteniendo el orden del dump).
func ParseInterfaceDump(raw []byte, preferred string) InterfaceDump {
	if len(raw) == 0 {
		return InterfaceDump{}
	}
	var dump struct {
		Interface []ifaceStatus `json:"interface"`
	}
	if err := json.Unmarshal(raw, &dump); err != nil || len(dump.Interface) == 0 {
		return InterfaceDump{}
	}
	var res InterfaceDump
	if s, ok := pickUplinkPreferring(dump.Interface, preferred); ok {
		res.Wan = wanInfoFrom(s)
	}
	out := make([]NetIface, 0, len(dump.Interface))
	for _, it := range dump.Interface {
		if it.Interface == "" || it.Dynamic {
			continue
		}
		if it.Interface == "loopback" || it.L3Device == "lo" || it.Device == "lo" {
			continue
		}
		var devs []string
		if it.L3Device != "" {
			devs = append(devs, it.L3Device)
		}
		if it.Device != "" && it.Device != it.L3Device {
			devs = append(devs, it.Device)
		}
		out = append(out, NetIface{
			Name:    it.Interface,
			Proto:   it.Proto,
			Up:      it.Up,
			Devices: devs,
		})
	}
	if len(out) > 0 {
		res.Ifaces = out
	}
	return res
}
