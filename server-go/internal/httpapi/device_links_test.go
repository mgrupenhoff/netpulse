package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gnacho/netpulse/server-go/internal/adapters"
	"github.com/gnacho/netpulse/server-go/internal/db"
)

// TestDeviceLinksRejectInvalidMAC comprueba que PUT /api/devices/{mac}/link
// rechaza MACs mal formadas en path o body con 400 invalid_mac.
func TestDeviceLinksRejectInvalidMAC(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer d.Close()
	s := &server{db: d}

	t.Run("PUT con path MAC inválida", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/devices/no-es-mac/link", strings.NewReader(`{"target":"AA:BB:CC:DD:EE:01"}`))
		req.SetPathValue("mac", "no-es-mac")
		rec := httptest.NewRecorder()
		s.handleDeviceLinkPut(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_mac") {
			t.Fatalf("esperaba 400 invalid_mac, obtuve %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("PUT con target MAC inválida", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/devices/AA:BB:CC:DD:EE:01/link", strings.NewReader(`{"target":"basura"}`))
		req.SetPathValue("mac", "AA:BB:CC:DD:EE:01")
		rec := httptest.NewRecorder()
		s.handleDeviceLinkPut(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_mac") {
			t.Fatalf("esperaba 400 invalid_mac, obtuve %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestDeviceLinkDeleteStaysLenient comprueba que DELETE /api/devices/{mac}/link
// sigue desenlazando una fila heredada cuya clave no es una MAC (como hace
// DELETE /api/devices/{mac}) y que una clave desconocida da 404.
func TestDeviceLinkDeleteStaysLenient(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer d.Close()
	s := &server{db: d}
	if _, err := d.Exec("INSERT INTO device_links (mac, canonical) VALUES (?, ?)", "LEGACY:ALIAS", "AA:BB:CC:DD:EE:01"); err != nil {
		t.Fatalf("sembrar device_links: %v", err)
	}

	del := func(raw string) int {
		req := httptest.NewRequest(http.MethodDelete, "/api/devices/x/link", nil)
		req.SetPathValue("mac", raw)
		rec := httptest.NewRecorder()
		s.handleDeviceLinkDelete(rec, req)
		return rec.Code
	}
	if code := del("no-existe"); code != http.StatusNotFound {
		t.Fatalf("clave desconocida: esperaba 404, obtuve %d", code)
	}
	if code := del("legacy-alias"); code != http.StatusNoContent {
		t.Fatalf("fila heredada: esperaba 204, obtuve %d", code)
	}
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM device_links").Scan(&n); err != nil || n != 0 {
		t.Fatalf("la fila heredada debía borrarse: n=%d err=%v", n, err)
	}
}

// TestHandleUsteerKickRejectsInvalidMAC comprueba que POST /api/usteer/{mac}/kick
// rechaza MACs vacías o mal formadas en la frontera HTTP con 400 invalid_mac.
func TestHandleUsteerKickRejectsInvalidMAC(t *testing.T) {
	s := &server{adapter: adapters.NewDemo()}
	for _, raw := range []string{"", "no-es-mac"} {
		req := httptest.NewRequest(http.MethodPost, "/api/usteer/"+raw+"/kick", nil)
		req.SetPathValue("mac", raw)
		rec := httptest.NewRecorder()
		s.handleUsteerKick(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_mac") {
			t.Fatalf("mac=%q: esperaba 400 invalid_mac, obtuve %d: %s", raw, rec.Code, rec.Body.String())
		}
	}
}
