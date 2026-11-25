package httpapi

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnacho/netpulse/server-go/internal/logonce"
)

// TestBeaconWarnsOncePerInvalidMAC: una entrada FDB y una MAC de puente que
// no son MAC se descartan y se avisan una sola vez por valor, aunque el
// switch repita el datagrama.
func TestBeaconWarnsOncePerInvalidMAC(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	old := beaconWarn
	beaconWarn = logonce.NewWithLogf("[beacon] ", func(f string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, fmt.Sprintf(f, a...))
	})
	t.Cleanup(func() { beaconWarn = old })

	s, token := newBeaconTestServer(t)
	addr, err := s.startBeaconListener("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listener: %v", err)
	}
	defer s.beaconConn.Close()

	for seq := 1; seq <= 3; seq++ {
		sendUDP(t, addr, fmt.Sprintf(
			`{"v":1,"seq":%d,"slug":"switch16","token":"%s","fdb":{"AABBCCDDEE01":"3","corta":"1"}}`, seq, token))
		sendUDP(t, addr, fmt.Sprintf(
			`{"v":1,"seq":%d,"slug":"switch16","token":"%s","ports":[{"n":1,"l":3,"tx":1,"rx":2}],`+
				`"up":12345,"mac":"mala","fw":"v0.1.0-aac5ea2"}`, seq+10, token))
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(lines)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond) // margen para que un aviso repetido llegara
	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 2 {
		t.Fatalf("avisos: %q (want uno por FDB y otro por la MAC del puente)", lines)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, `beacon.fdb router=switch16: ignoring invalid MAC "corta"`) ||
		!strings.Contains(joined, `beacon.bridge router=switch16: ignoring invalid MAC "mala"`) {
		t.Fatalf("avisos: %q", lines)
	}
}
