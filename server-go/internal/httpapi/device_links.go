package httpapi

// device_links.go — enlace manual de clientes con varias MACs (#1151):
// "esta MAC es el mismo dispositivo que esta otra". Un cliente canónico
// agrupa a sus alias en la lista (mergeLinkedDevices en el adapter).

import (
	"net/http"
	"strings"

	"github.com/gnacho/netpulse/agent/macaddr"
)

// handleDeviceLinkPut: body {"target": "<mac>"} — enlaza la MAC {mac} como
// alias del cliente {target}. Reglas: no enlaces a sí mismo, ni alias ya
// enlazada (desenlazar primero), ni target que sea a su vez alias (sin
// cadenas: el merge resuelve un solo nivel a propósito).
func (s *server) handleDeviceLinkPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Target string `json:"target"`
	}
	if st := readJSONBody(w, r, &body); st != 0 {
		writeBodyError(w, st, "invalid_body", "body JSON inválido")
		return
	}
	alias, err := macaddr.Parse(r.PathValue("mac"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_mac", "invalid mac")
		return
	}
	target, err := macaddr.Parse(body.Target)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_mac", "invalid target mac")
		return
	}
	if alias == target {
		writeError(w, http.StatusBadRequest, "invalid_input", "cannot link a MAC to itself")
		return
	}
	links := loadDeviceLinks(s.db)
	if _, already := links[alias]; already {
		writeError(w, http.StatusConflict, "conflict", "this MAC is already linked; unlink it first")
		return
	}
	if _, targetIsAlias := links[target]; targetIsAlias {
		writeError(w, http.StatusConflict, "conflict", "target is itself a linked alias; link to the canonical MAC")
		return
	}
	if _, err := s.db.Exec(
		"INSERT INTO device_links (mac, canonical) VALUES (?, ?) ON CONFLICT(mac) DO UPDATE SET canonical = excluded.canonical",
		alias, target,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mac": alias, "canonical": target})
}

// handleDeviceLinkDelete: desenlaza la alias {mac} (vuelve a ser un cliente
// independiente en la siguiente construcción de la lista). A diferencia del
// PUT, tolera claves que no son MAC: una fila heredada con basura en la tabla
// tiene que poder desenlazarse.
func (s *server) handleDeviceLinkDelete(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("mac")
	var alias any
	if m, err := macaddr.Parse(raw); err == nil {
		alias = m
	} else {
		// Una fila heredada puede tener como clave algo que no es una MAC
		// (la tabla es texto libre). Se borra por esa clave cruda, con la
		// misma normalización laxa que DELETE /api/devices/{mac}, para que
		// siga siendo posible desenlazarla.
		key := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(raw), "-", ":"))
		if key == "" {
			writeError(w, http.StatusBadRequest, "invalid_mac", "invalid mac")
			return
		}
		alias = key
	}
	res, err := s.db.Exec("DELETE FROM device_links WHERE mac = ?", alias)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// kvRackUplinkUnit: unidad designada como central del auto-cableado (#1186).
// Su FDB es la fuente de verdad de los cables detectados.
const kvRackUplinkUnit = "rack_uplink_unit"

func (s *server) uplinkUnitID() string {
	v := kvGet(s.db.DB, kvRackUplinkUnit)
	return strings.TrimSpace(v)
}

// handleRacksUplinkUnitGet: devuelve la unidad designada ("" = sin designar).
func (s *server) handleRacksUplinkUnitGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"id": s.uplinkUnitID()})
}

// handleRacksUplinkUnitPut: body {"id": "<routerId>"} — designa la unidad
// cuyo FDB alimenta el auto-cableado. Vacío = sin designar.
func (s *server) handleRacksUplinkUnitPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if st := readJSONBody(w, r, &body); st != 0 {
		writeBodyError(w, st, "invalid_body", "body JSON inválido")
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "missing id")
		return
	}
	found := false
	for _, rt := range s.adapter.GetRouters(r.Context()) {
		if rt.ID == id {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "unknown unit id")
		return
	}
	if _, err := s.db.Exec(`INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		kvRackUplinkUnit, id); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

// handleRacksUplinkUnitDelete: quita la designación (solo cables manuales).
func (s *server) handleRacksUplinkUnitDelete(w http.ResponseWriter, r *http.Request) {
	if _, err := s.db.Exec(`DELETE FROM kv WHERE key = ?`, kvRackUplinkUnit); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
