package adapters

import (
	"reflect"
	"testing"

	"github.com/gnacho/netpulse/agent/macaddr"
)

// #1190: los hints de flota salen del FDB de la unidad designada. Cada MAC
// bridge de flota que aprende es un cable; el puerto del hijo es donde el hijo
// aprende a la central, sin distinguir mayúsculas en el FDB.
func TestFleetFdbUplinksFromDesignatedUnit(t *testing.T) {
	var (
		macGW  = macaddr.MustParse("AA:BB:CC:00:00:01")
		macSw  = macaddr.MustParse("AA:BB:CC:00:00:02")
		macAP  = macaddr.MustParse("AA:BB:CC:00:00:03")
		macCli = macaddr.MustParse("10:00:00:00:00:01")
	)
	polled := map[string]*routerPolled{
		"gw": {brMac: macGW, fdb: map[macaddr.MAC]string{macSw: "lan1"}},
		"sw": {brMac: macSw, fdb: map[macaddr.MAC]string{
			macaddr.MustParse("aa:bb:cc:00:00:01"): "port1", // gateway (clave en minúsculas)
			macAP:                                  "port5",
			macCli:                                 "port7", // cliente: no es de flota
			macSw:                                  "cpu",   // la propia MAC bridge
		}},
		"ap": {brMac: macAP, fdb: map[macaddr.MAC]string{macaddr.MustParse("aa:bb:cc:00:00:02"): "lan"}},
	}
	l := &Live{lastPolled: polled}

	got := l.FleetFdbUplinks("sw", macaddr.MAC{})
	want := map[string]FdbUplink{
		"gw": {ChildID: "gw", ParentID: "sw", Port: "port1", ChildPort: "lan1"},
		"ap": {ChildID: "ap", ParentID: "sw", Port: "port5", ChildPort: "lan"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FleetFdbUplinks = %+v, esperaba %+v", got, want)
	}

	// Sin MAC bridge en el sondeo de la central, infraMAC la sustituye para
	// el puerto del hijo.
	polled["sw"].brMac = macaddr.MAC{}
	got = l.FleetFdbUplinks("sw", macaddr.MustParse("aa:bb:cc:00:00:02"))
	if got["ap"].ChildPort != "lan" {
		t.Errorf("ap.ChildPort = %q, esperaba lan (con infraMAC)", got["ap"].ChildPort)
	}

	for _, id := range []string{"", "nope"} {
		if got := l.FleetFdbUplinks(id, macaddr.MAC{}); len(got) != 0 {
			t.Errorf("central %q: %+v, esperaba vacío", id, got)
		}
	}
}
