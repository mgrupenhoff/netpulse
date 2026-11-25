package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gnacho/netpulse/server-go/internal/db"
	"github.com/gnacho/netpulse/server-go/internal/rack"
)

// TestWriteRackStoreErrorInvalidMAC comprueba que una MAC inválida en el layout
// es un error del cliente (400 invalid_mac) y no un fallo de base de datos.
func TestWriteRackStoreErrorInvalidMAC(t *testing.T) {
	rec := httptest.NewRecorder()
	writeRackStoreError(rec, rack.ErrInvalidMAC)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandleRackLayoutPutRejectsInvalidMAC comprueba que una device_mac
// mal formada en el JSON de layout se rechaza en la frontera HTTP con 400.
func TestHandleRackLayoutPutRejectsInvalidMAC(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer d.Close()

	r, err := rack.CreateRack(d.DB, rack.Rack{Name: "lab", UHeight: 12})
	if err != nil {
		t.Fatalf("CreateRack: %v", err)
	}
	s := &server{db: d}
	body := `{"rack_id":"` + r.ID + `","upsert":[{"device_mac":"no-es-una-mac","u_start":1,"col_start":0}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/racks/layout", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleRackLayoutPut(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_mac") {
		t.Fatalf("esperaba código invalid_mac, obtuve %s", rec.Body.String())
	}
}
