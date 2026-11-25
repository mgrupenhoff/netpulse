package adapters

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gnacho/netpulse/server-go/internal/logonce"
)

// TestRecordClientBwSamplesWarnsOncePerInvalidMAC: una muestra con MAC que no
// lo es se descarta y se avisa una sola vez por valor; la MAC vacía no avisa.
func TestRecordClientBwSamplesWarnsOncePerInvalidMAC(t *testing.T) {
	var lines []string
	old := bwWarn
	bwWarn = logonce.NewWithLogf("[clientbw] ", func(f string, a ...any) {
		lines = append(lines, fmt.Sprintf(f, a...))
	})
	t.Cleanup(func() { bwWarn = old })

	d := openLiveTestDB(t)
	l := &Live{db: d}
	t0 := time.Now()
	samples := []clientBwSample{
		{mac: "xyz", rx: 1, tx: 1},
		{mac: "", rx: 1, tx: 1},
		{mac: "AA:BB:CC:DD:EE:01", rx: 1, tx: 1},
	}
	for i := 0; i < 3; i++ {
		l.recordClientBwSamples("gw", t0.Add(time.Duration(i)*time.Second), samples, "hostapd")
	}
	if len(lines) != 1 || !strings.Contains(lines[0], `"xyz"`) || !strings.Contains(lines[0], "router=gw") {
		t.Fatalf("avisos: %q (want uno que cite el valor y el router)", lines)
	}
}
