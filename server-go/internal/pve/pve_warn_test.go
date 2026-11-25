package pve

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gnacho/netpulse/server-go/internal/logonce"
)

// TestNICsOfConfigWarnsOncePerInvalidHwaddr: una hwaddr que no es MAC descarta
// el NIC y se avisa una sola vez por valor aunque el inventario se lea
// en cada sondeo.
func TestNICsOfConfigWarnsOncePerInvalidHwaddr(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	old := pveWarn
	pveWarn = logonce.NewWithLogf("[pve] ", func(f string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, fmt.Sprintf(f, a...))
	})
	t.Cleanup(func() { pveWarn = old })

	cfg := map[string]any{
		"net0": "bridge=vmbr0,hwaddr=xyz,ip=dhcp",
		"net1": "virtio=BC:24:11:00:00:02,bridge=vmbr1",
	}
	for i := 0; i < 3; i++ {
		nics := NICsOfConfig(cfg)
		if len(nics) != 1 || nics[0].MAC != "BC:24:11:00:00:02" {
			t.Fatalf("nics: %+v", nics)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 1 || !strings.Contains(lines[0], `"xyz"`) || !strings.Contains(lines[0], "net0") {
		t.Fatalf("avisos: %q (want uno que cite net0 y el valor)", lines)
	}
}
