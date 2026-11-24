package probe

import (
	"context"
	"reflect"
	"testing"
)

const travelNetworkDump = `{"interface":[
  {"interface":"lan","up":true,"proto":"static","l3_device":"br-lan","device":"br-lan","dynamic":false},
  {"interface":"loopback","up":true,"proto":"static","l3_device":"lo","device":"lo","dynamic":false},
  {"interface":"wan","up":false,"proto":"dhcp","l3_device":null,"device":"eth0.2","dynamic":false},
  {"interface":"wan6","up":false,"proto":"dhcpv6","l3_device":null,"device":"eth0.2","dynamic":false},
  {"interface":"wan_4","up":true,"proto":"dhcp","l3_device":"eth0.2","device":"eth0.2","dynamic":true},
  {"interface":"wg0","up":true,"proto":"wireguard","l3_device":"wg0","device":null,"dynamic":false},
  {"interface":"wwan","up":true,"proto":"dhcp","l3_device":"phy1-sta0","device":"phy1-sta0","dynamic":false,"ipv4-address":[{"address":"10.20.0.177","mask":24}],"route":[{"target":"0.0.0.0","mask":0,"nexthop":"10.20.0.1"}]}
]}`

func TestParseInterfaceDumpTravelRouter(t *testing.T) {
	got := ParseInterfaceDump([]byte(travelNetworkDump), "").Ifaces
	want := []NetIface{
		{Name: "lan", Proto: "static", Up: true, Devices: []string{"br-lan"}},
		{Name: "wan", Proto: "dhcp", Up: false, Devices: []string{"eth0.2"}},
		{Name: "wan6", Proto: "dhcpv6", Up: false, Devices: []string{"eth0.2"}},
		{Name: "wg0", Proto: "wireguard", Up: true, Devices: []string{"wg0"}},
		{Name: "wwan", Proto: "dhcp", Up: true, Devices: []string{"phy1-sta0"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseInterfaceDump:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestParseInterfaceDumpEdgeCases(t *testing.T) {
	t.Run("l3_device y device distintos conserva ambos en orden", func(t *testing.T) {
		raw := `{"interface":[{"interface":"wan","up":true,"proto":"pppoe","l3_device":"pppoe-wan","device":"eth0.2","dynamic":false}]}`
		got := ParseInterfaceDump([]byte(raw), "").Ifaces
		want := []NetIface{
			{Name: "wan", Proto: "pppoe", Up: true, Devices: []string{"pppoe-wan", "eth0.2"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("pppoe:\ngot  %+v\nwant %+v", got, want)
		}
	})

	t.Run("json invalido o vacio devuelve nil", func(t *testing.T) {
		if got := ParseInterfaceDump([]byte("{roto"), "").Ifaces; got != nil {
			t.Fatalf("JSON inválido debía devolver nil: %+v", got)
		}
		if got := ParseInterfaceDump([]byte(`{"interface":[{"interface":"loopback","up":true,"l3_device":"lo","device":"lo"}]}`), "").Ifaces; got != nil {
			t.Fatalf("solo loopback debía devolver nil: %+v", got)
		}
		if got := ParseInterfaceDump(nil, "").Ifaces; got != nil {
			t.Fatalf("entrada nil debía devolver nil: %+v", got)
		}
	})
}

func TestProberBuildReportsNetIfaces(t *testing.T) {
	t.Run("router con uplink", func(t *testing.T) {
		p := NewProber(fakeRunner{outs: map[string]string{
			CmdNetworkDump: travelNetworkDump,
		}}, Options{ScanInterval: ScanDisabled})
		pl := p.Build(context.Background(), "travel", "test")
		if pl.Data.Wan == nil || pl.Data.Wan.Device != "phy1-sta0" {
			t.Fatalf("esperaba Wan en phy1-sta0, obtuve %+v", pl.Data.Wan)
		}
		if len(pl.Data.NetIfaces) != 5 || pl.Data.NetIfaces[0].Name != "lan" || pl.Data.NetIfaces[4].Name != "wwan" {
			t.Fatalf("NetIfaces inesperadas en Build: %+v", pl.Data.NetIfaces)
		}
	})

	t.Run("ap sin uplink sigue reportando sus redes", func(t *testing.T) {
		apDump := `{"interface":[
			{"interface":"loopback","up":true,"proto":"static","l3_device":"lo","device":"lo","dynamic":false},
			{"interface":"lan","up":true,"proto":"dhcp","l3_device":"br-lan","device":"br-lan","dynamic":false}
		]}`
		p := NewProber(fakeRunner{outs: map[string]string{
			CmdNetworkDump: apDump,
		}}, Options{ScanInterval: ScanDisabled})
		pl := p.Build(context.Background(), "ap", "test")
		if pl.Data.Wan != nil {
			t.Fatalf("un AP sin uplink no debía reportar Wan: %+v", pl.Data.Wan)
		}
		want := []NetIface{{Name: "lan", Proto: "dhcp", Up: true, Devices: []string{"br-lan"}}}
		if !reflect.DeepEqual(pl.Data.NetIfaces, want) {
			t.Fatalf("NetIfaces del AP:\ngot  %+v\nwant %+v", pl.Data.NetIfaces, want)
		}
	})
}
