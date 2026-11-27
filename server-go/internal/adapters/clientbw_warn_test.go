package adapters

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gnacho/netpulse/agent/macaddr"
	"github.com/gnacho/netpulse/agent/probe"
	"github.com/gnacho/netpulse/server-go/internal/logonce"
)

// TestResolveClientBwSourcesWarnsOncePerInvalidMAC: un host de nlbwmon cuya MAC
// no lo es se descarta y se avisa una sola vez por valor, citando el router;
// la MAC vacía no avisa y las válidas siguen pasando.
func TestResolveClientBwSourcesWarnsOncePerInvalidMAC(t *testing.T) {
	var lines []string
	old := bwWarn
	bwWarn = logonce.NewWithLogf("[clientbw] ", func(f string, a ...any) {
		lines = append(lines, fmt.Sprintf(f, a...))
	})
	t.Cleanup(func() { bwWarn = old })

	nlbw := &probe.ClientBwData{Available: true, Hosts: map[string]probe.NlbwCounter{
		"xyz":               {RxBytes: 1, TxBytes: 1},
		"":                  {RxBytes: 1, TxBytes: 1},
		"AA:BB:CC:DD:EE:01": {RxBytes: 1, TxBytes: 1},
	}}
	want := macaddr.MustParse("AA:BB:CC:DD:EE:01")
	for i := 0; i < 3; i++ {
		samples, source, ok := resolveClientBwSources("gw", nil, nlbw)
		if !ok || source != "nlbwmon" || len(samples) != 1 || samples[0].mac != want {
			t.Fatalf("ok=%v source=%s samples=%+v (want solo la muestra válida)", ok, source, samples)
		}
	}
	if len(lines) != 1 || !strings.Contains(lines[0], `ignoring invalid MAC "xyz"`) || !strings.Contains(lines[0], "router=gw") {
		t.Fatalf("avisos: %q (want uno que cite el valor y el router)", lines)
	}
}
