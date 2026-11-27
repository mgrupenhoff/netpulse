package adapters

import (
	"reflect"
	"testing"

	"github.com/gnacho/netpulse/agent/macaddr"
)

// #1942: un cliente cae en exactamente una red o en ninguna. Los casos
// cubren el br-lan sin VLAN (antes la columna quedaba vacia), el split
// lan/guest, el trunk que no se puede adivinar y la via WiFi.
func TestAttributeNetworkClients_1942(t *testing.T) {
	macAA := macaddr.MustParse("AA:00:00:00:00:01")
	macBB := macaddr.MustParse("BB:00:00:00:00:02")
	lan := NetworkInfo{Name: "lan", Device: "br-lan", Ports: []string{"lan1", "lan2", "lan3"}}
	guest := NetworkInfo{Name: "guest", Device: "br-guest", Vlan: 20, Ports: []string{"lan3"}}
	iot := NetworkInfo{Name: "iot", Device: "br-iot", Vlan: 999, Ports: []string{"eth0.999"}}
	cases := []struct {
		name     string
		networks []NetworkInfo
		clients  []Device
		wifiNet  map[string]string
		want     map[string][]macaddr.MAC // red -> MACs; las redes sin entrada quedan sin el campo
	}{
		{
			name:     "br-lan sin VLAN atribuye por puerto",
			networks: []NetworkInfo{lan},
			clients: []Device{
				{MAC: macAA, Port: "lan2"},
				{MAC: macBB}, // sin puerto ni VLAN: no se adivina
			},
			want: map[string][]macaddr.MAC{"lan": {macAA}},
		},
		{
			name:     "puerto trunk con VLAN desempata",
			networks: []NetworkInfo{lan, guest},
			clients:  []Device{{MAC: macAA, Port: "lan3", Vlan: 20}},
			want:     map[string][]macaddr.MAC{"guest": {macAA}},
		},
		{
			name:     "puerto trunk sin VLAN no atribuye",
			networks: []NetworkInfo{lan, guest},
			clients:  []Device{{MAC: macAA, Port: "lan3"}},
			want:     map[string][]macaddr.MAC{},
		},
		{
			name:     "puerto exclusivo sin VLAN si atribuye",
			networks: []NetworkInfo{lan, guest},
			clients:  []Device{{MAC: macAA, Port: "lan1"}},
			want:     map[string][]macaddr.MAC{"lan": {macAA}},
		},
		{
			name:     "subinterfaz: base del puerto mas VLAN",
			networks: []NetworkInfo{iot},
			clients:  []Device{{MAC: macAA, Port: "eth0", Vlan: 999}},
			want:     map[string][]macaddr.MAC{"iot": {macAA}},
		},
		{
			name:     "VLAN sola, sin puerto, como antes",
			networks: []NetworkInfo{lan, guest},
			clients:  []Device{{MAC: macAA, Vlan: 20}},
			want:     map[string][]macaddr.MAC{"guest": {macAA}},
		},
		{
			name:     "wifi por config wireless",
			networks: []NetworkInfo{lan, guest},
			clients: []Device{
				{MAC: macAA, WifiIface: "wlan1"},
				{MAC: macBB, WifiIface: "wlan9"}, // iface no declarada: ninguna
			},
			wifiNet: map[string]string{"wlan1": "guest"},
			want:    map[string][]macaddr.MAC{"guest": {macAA}},
		},
		{
			name:     "MAC vacia nunca se lista",
			networks: []NetworkInfo{lan},
			clients:  []Device{{MAC: macaddr.MAC{}, Port: "lan1"}},
			want:     map[string][]macaddr.MAC{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := attributeNetworkClients(tc.networks, tc.clients, tc.wifiNet)
			if len(got) != len(tc.networks) {
				t.Fatalf("redes devueltas = %d, quiero %d", len(got), len(tc.networks))
			}
			for _, n := range got {
				want := tc.want[n.Name]
				if !reflect.DeepEqual(n.ClientMacs, want) && !(len(n.ClientMacs) == 0 && len(want) == 0) {
					t.Errorf("red %s: ClientMacs = %v, quiero %v", n.Name, n.ClientMacs, want)
				}
			}
		})
	}
}

// La entrada no se modifica: el cache anti-parpadeo comparte el slice.
func TestAttributeNetworkClientsNoMutaEntrada(t *testing.T) {
	in := []NetworkInfo{{Name: "lan", Ports: []string{"lan1"}}}
	attributeNetworkClients(in, []Device{{MAC: macaddr.MustParse("AA:00:00:00:00:01"), Port: "lan1"}}, nil)
	if len(in[0].ClientMacs) != 0 {
		t.Fatalf("la entrada quedo modificada: %v", in[0].ClientMacs)
	}
}

func TestWifiNetworkMap(t *testing.T) {
	ifaces := []Dot11rIface{
		{Ifname: "wlan0", Network: "lan"},
		{Ifname: "wlan1", Network: "guest"},
		{Ifname: "", Network: "lan"},   // sin ifname no mapea
		{Ifname: "wlan2", Network: ""}, // sin red no mapea
	}
	got := wifiNetworkMap(ifaces)
	want := map[string]string{"wlan0": "lan", "wlan1": "guest"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wifiNetworkMap = %v, quiero %v", got, want)
	}
	if wifiNetworkMap([]Dot11rIface{{Ifname: "wlan0"}}) != nil {
		t.Fatal("sin mapeos utiles debe devolver nil")
	}
}
