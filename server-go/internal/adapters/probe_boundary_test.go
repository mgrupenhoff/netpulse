package adapters

import (
	"bytes"
	"log"
	"reflect"
	"strings"
	"testing"

	"github.com/gnacho/netpulse/agent/macaddr"
	"github.com/gnacho/netpulse/agent/probe"
)

// TestProbeBoundaryConversionsParseAndSkipInvalidMACs comprueba que las
// conversiones de la frontera del sondeo (leases, reservas, discovery)
// parsean las MACs válidas, descartan las inválidas o vacías y avisan de las
// inválidas en el log.
func TestProbeBoundaryConversionsParseAndSkipInvalidMACs(t *testing.T) {
	var buf bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prevOut)

	exp := int64(1700000123)
	if leasesFromProbe("test.lease", nil) != nil {
		t.Fatal("leasesFromProbe(nil) debe devolver nil")
	}
	emptyLeases := leasesFromProbe("test.lease", []probe.DhcpLease{})
	if emptyLeases == nil || len(emptyLeases) != 0 {
		t.Fatalf("leasesFromProbe(vacío) = %#v, esperaba slice no nil vacío", emptyLeases)
	}
	gotLeases := leasesFromProbe("test.lease", []probe.DhcpLease{
		{MAC: "aa-bb-cc-dd-ee-01", IP: "192.168.1.10", Hostname: "pc", VendorClass: "msft", ClientID: "01:aa", LeaseExpiresAt: &exp},
		{MAC: "not-a-mac", IP: "192.168.1.11", Hostname: "bad"},
		{MAC: "", IP: "192.168.1.12", Hostname: "empty"},
	})
	wantLeases := []DhcpLease{
		{MAC: macaddr.MustParse("AA:BB:CC:DD:EE:01"), IP: "192.168.1.10", Hostname: "pc", VendorClass: "msft", ClientID: "01:aa", LeaseExpiresAt: &exp},
	}
	if !reflect.DeepEqual(gotLeases, wantLeases) {
		t.Fatalf("leasesFromProbe = %+v, want %+v", gotLeases, wantLeases)
	}

	if reservationsFromProbe(nil) != nil {
		t.Fatal("reservationsFromProbe(nil) debe devolver nil")
	}
	gotRes := reservationsFromProbe([]probe.DhcpReservation{
		{MAC: "aa:bb:cc:dd:ee:02", Name: "nas", IP: "192.168.1.20"},
		{MAC: "bogus-res", Name: "bad", IP: "192.168.1.21"},
	})
	wantRes := []DhcpReservation{
		{MAC: macaddr.MustParse("AA:BB:CC:DD:EE:02"), Name: "nas", IP: "192.168.1.20"},
	}
	if !reflect.DeepEqual(gotRes, wantRes) {
		t.Fatalf("reservationsFromProbe = %+v, want %+v", gotRes, wantRes)
	}

	if discoveryFromProbe(nil) != nil {
		t.Fatal("discoveryFromProbe(nil) debe devolver nil")
	}
	gotDisc := discoveryFromProbe(&probe.DiscoveryData{
		Services:      map[string][]string{"apple-tv.local": {"_airplay._tcp"}},
		HostByIP:      map[string]string{"192.168.1.30": "apple-tv.local"},
		Models:        map[string]string{"apple-tv.local": "AppleTV"},
		Manufacturers: map[string]string{"apple-tv.local": "Apple"},
		RandomMACs:    []string{"02:11:22:33:44:55", "bad-random"},
	})
	if len(gotDisc.RandomMACs) != 1 || gotDisc.RandomMACs[0] != macaddr.MustParse("02:11:22:33:44:55") {
		t.Fatalf("discoveryFromProbe.RandomMACs = %+v", gotDisc.RandomMACs)
	}
	if gotDisc.HostByIP["192.168.1.30"] != "apple-tv.local" || gotDisc.Models["apple-tv.local"] != "AppleTV" {
		t.Fatalf("discoveryFromProbe fields = %+v", gotDisc)
	}

	logs := buf.String()
	for _, want := range []string{"test.lease", "not-a-mac", "dhcp.reservation", "bogus-res", "discovery.random_mac", "bad-random"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("esperaba %q en log, obtuve %q", want, logs)
		}
	}
}
