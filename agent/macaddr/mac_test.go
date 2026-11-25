package macaddr

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	const want = "AA:BB:CC:DD:EE:FF"
	wantBytes := [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}

	tests := []struct {
		name   string
		in     string
		want   string
		wantOK bool
		wantB  [6]byte
	}{
		{"dos puntos en minúsculas", "aa:bb:cc:dd:ee:ff", want, true, wantBytes},
		{"dos puntos en mayúsculas", "AA:BB:CC:DD:EE:FF", want, true, wantBytes},
		{"con guiones", "aa-bb-cc-dd-ee-ff", want, true, wantBytes},
		{"12 hex sin separador", "aabbccddeeff", want, true, wantBytes},
		{"espacios alrededor", "  aa:bb:cc:dd:ee:ff\t\n", want, true, wantBytes},
		{"todos ceros válida y distinta del valor cero", "00:00:00:00:00:00", "00:00:00:00:00:00", true, [6]byte{}},
		{"separadores mezclados", "aa:bb-cc:dd-ee:ff", "", false, [6]byte{}},
		{"separador incorrecto", "aa.bb.cc.dd.ee.ff", "", false, [6]byte{}},
		{"cisco con puntos", "aabb.ccdd.eeff", "", false, [6]byte{}},
		{"no hex", "gg:bb:cc:dd:ee:ff", "", false, [6]byte{}},
		{"no hex sin separador", "aabbccddeefg", "", false, [6]byte{}},
		{"demasiado corta", "aa:bb:cc:dd:ee", "", false, [6]byte{}},
		{"demasiado larga", "aa:bb:cc:dd:ee:ff:00", "", false, [6]byte{}},
		{"vacía", "", "", false, [6]byte{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("Parse(%q) devolvió error inesperado: %v", tc.in, err)
				}
				if !got.IsValid() {
					t.Errorf("Parse(%q).IsValid() = false, esperaba true", tc.in)
				}
				if got.String() != tc.want {
					t.Errorf("Parse(%q).String() = %q, esperaba %q", tc.in, got.String(), tc.want)
				}
				if got.As6() != tc.wantB {
					t.Errorf("Parse(%q).As6() = %x, esperaba %x", tc.in, got.As6(), tc.wantB)
				}
			} else {
				if !errors.Is(err, ErrSyntax) {
					t.Fatalf("Parse(%q) = (%q, %v), esperaba ErrSyntax", tc.in, got, err)
				}
				if got.IsValid() {
					t.Errorf("Parse(%q) con error devolvió MAC válida", tc.in)
				}
			}
		})
	}
}

func TestParseSeparatedAndColon(t *testing.T) {
	if m, err := ParseSeparated("aa-bb-cc-dd-ee-ff"); err != nil || m.String() != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("ParseSeparated con guiones falló: %v, %v", m, err)
	}
	if _, err := ParseSeparated("aabbccddeeff"); err == nil {
		t.Error("ParseSeparated aceptó 12 hex sin separador")
	}
	if m, err := ParseColon("  aa:bb:cc:dd:ee:ff  "); err != nil || m.String() != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("ParseColon con ':' falló: %v, %v", m, err)
	}
	for _, bad := range []string{"aa-bb-cc-dd-ee-ff", "cafebabe1234", "aa:bb-cc:dd:ee:ff", ""} {
		if _, err := ParseColon(bad); err == nil {
			t.Errorf("ParseColon(%q) debería fallar", bad)
		}
	}
}

func TestZeroValueAndEquality(t *testing.T) {
	var zero MAC
	if zero.IsValid() {
		t.Error("valor cero no debe ser válido")
	}
	if zero.String() != "" || zero.Lower() != "" || zero.DeviceID() != "" {
		t.Errorf("valor cero devolvió texto no vacío: %q / %q / %q", zero.String(), zero.Lower(), zero.DeviceID())
	}
	if zero.IsLocallyAdministered() {
		t.Error("valor cero no debe ser administrado localmente")
	}

	allZero := MustParse("00:00:00:00:00:00")
	if !allZero.IsValid() {
		t.Error("00:00:00:00:00:00 debe ser válida")
	}
	if zero == allZero {
		t.Error("valor cero no debe igualar a 00:00:00:00:00:00")
	}

	// Dos entradas equivalentes con distinta caja y separador deben producir la
	// misma MAC para == y map[MAC]T.
	a := MustParse("aa-bb-cc-dd-ee-ff")
	b := MustParse("AA:BB:CC:DD:EE:FF")
	c := From6([6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF})
	if a != b || b != c {
		t.Errorf("MACs equivalentes no son iguales: %#v / %#v / %#v", a, b, c)
	}
	byMAC := map[MAC]string{a: "ok"}
	if byMAC[b] != "ok" || byMAC[c] != "ok" {
		t.Error("lookup en map[MAC]string falló para formas equivalentes")
	}
}

func TestFormatHelpersAndLAA(t *testing.T) {
	m := MustParse("0a:1b:2c:3d:4e:5f")
	if m.String() != "0A:1B:2C:3D:4E:5F" {
		t.Errorf("String() = %q", m.String())
	}
	if m.Lower() != "0a:1b:2c:3d:4e:5f" {
		t.Errorf("Lower() = %q", m.Lower())
	}
	if m.DeviceID() != "0a-1b-2c-3d-4e-5f" {
		t.Errorf("DeviceID() = %q", m.DeviceID())
	}
	if !m.IsLocallyAdministered() {
		t.Error("0A:... debe marcar IsLocallyAdministered = true")
	}
	for _, local := range []string{"02:11:22:33:44:55", "06:11:22:33:44:55", "0E:11:22:33:44:55", "F2:11:22:33:44:55"} {
		if !MustParse(local).IsLocallyAdministered() {
			t.Errorf("%s debe marcar IsLocallyAdministered = true", local)
		}
	}
	for _, univ := range []string{"00:11:22:33:44:55", "04:11:22:33:44:55", "AC:11:22:33:44:55"} {
		if MustParse(univ).IsLocallyAdministered() {
			t.Errorf("%s no debe marcar IsLocallyAdministered = true", univ)
		}
	}
}

func TestCompare(t *testing.T) {
	in := []MAC{
		MustParse("FF:00:00:00:00:00"),
		{},
		MustParse("00:00:00:00:00:00"),
		MustParse("0A:00:00:00:00:00"),
		MustParse("09:FF:FF:FF:FF:FF"),
	}
	slices.SortFunc(in, MAC.Compare)
	want := []string{"", "00:00:00:00:00:00", "09:FF:FF:FF:FF:FF", "0A:00:00:00:00:00", "FF:00:00:00:00:00"}
	for i, w := range want {
		if in[i].String() != w {
			t.Errorf("índice %d = %q, esperaba %q", i, in[i].String(), w)
		}
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustParse con entrada inválida debería entrar en pánico")
		}
	}()
	_ = MustParse("no-es-mac")
}

func TestJSONRoundTripAndOmitZero(t *testing.T) {
	type payload struct {
		Req MAC            `json:"req"`
		Opt MAC            `json:"opt,omitzero"`
		Map map[MAC]string `json:"map,omitempty"`
	}

	// Valor cero en campo omitzero se omite; en requerido sale "".
	raw, err := json.Marshal(payload{})
	if err != nil {
		t.Fatalf("Marshal vacío: %v", err)
	}
	if string(raw) != `{"req":""}` {
		t.Errorf("Marshal vacío = %s, esperaba {\"req\":\"\"}", raw)
	}

	// Unmarshal normaliza campos y claves de mapa, preservando nil vs {}.
	var decNil, decEmpty, decFull payload
	if err := json.Unmarshal([]byte(`{"req":"aa-bb-cc-dd-ee-ff"}`), &decNil); err != nil {
		t.Fatalf("Unmarshal sin mapa: %v", err)
	}
	if decNil.Req != MustParse("AA:BB:CC:DD:EE:FF") || decNil.Opt.IsValid() || decNil.Map != nil {
		t.Errorf("decNil inesperado: %#v", decNil)
	}

	if err := json.Unmarshal([]byte(`{"req":"","map":{}}`), &decEmpty); err != nil {
		t.Fatalf("Unmarshal mapa vacío: %v", err)
	}
	if decEmpty.Req.IsValid() || decEmpty.Map == nil || len(decEmpty.Map) != 0 {
		t.Errorf("decEmpty debe conservar mapa no-nil vacío: %#v", decEmpty)
	}

	if err := json.Unmarshal([]byte(`{"req":"112233445566","opt":"aa:bb:cc:dd:ee:ff","map":{"aa-bb-cc-dd-ee-ff":"lan1"}}`), &decFull); err != nil {
		t.Fatalf("Unmarshal completo: %v", err)
	}
	if decFull.Req.String() != "11:22:33:44:55:66" || decFull.Opt.String() != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("campos inesperados: %#v", decFull)
	}
	if decFull.Map[MustParse("AA:BB:CC:DD:EE:FF")] != "lan1" {
		t.Errorf("clave de mapa no normalizada: %#v", decFull.Map)
	}

	// Texto inválido en JSON devuelve error.
	var bad payload
	if err := json.Unmarshal([]byte(`{"req":"not-a-mac"}`), &bad); err == nil {
		t.Error("Unmarshal de MAC inválida debería fallar")
	}
}

func TestSQLScanAndValue(t *testing.T) {
	var m MAC
	if err := m.Scan(nil); err != nil || m.IsValid() {
		t.Fatalf("Scan(nil) = (%v, %v), esperaba valor cero sin error", m, err)
	}
	if err := m.Scan(""); err != nil || m.IsValid() {
		t.Fatalf("Scan(\"\") = (%v, %v), esperaba valor cero sin error", m, err)
	}
	if err := m.Scan("aa-bb-cc-dd-ee-ff"); err != nil || m.String() != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("Scan(string) = (%q, %v)", m, err)
	}
	if err := m.Scan([]byte("112233445566")); err != nil || m.String() != "11:22:33:44:55:66" {
		t.Fatalf("Scan([]byte) = (%q, %v)", m, err)
	}
	if v, err := m.Value(); err != nil || v != "11:22:33:44:55:66" {
		t.Fatalf("Value() = (%v, %v)", v, err)
	}
	if err := m.Scan("basura"); err == nil {
		t.Error("Scan de texto no MAC debería fallar")
	}
	if err := m.Scan(123); err == nil {
		t.Error("Scan de int debería fallar")
	}
}

func TestZeroAllocsOnValidInput(t *testing.T) {
	raw := []byte("aa:bb:cc:dd:ee:ff")
	var m MAC
	if allocs := testing.AllocsPerRun(100, func() {
		if _, err := Parse("aa:bb:cc:dd:ee:ff"); err != nil {
			t.Fatal(err)
		}
	}); allocs != 0 {
		t.Errorf("Parse reservó %.1f veces, esperaba 0", allocs)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if err := m.UnmarshalText(raw); err != nil {
			t.Fatal(err)
		}
	}); allocs != 0 {
		t.Errorf("UnmarshalText reservó %.1f veces, esperaba 0", allocs)
	}
	var rawAny any = raw
	if allocs := testing.AllocsPerRun(100, func() {
		if err := m.Scan(rawAny); err != nil {
			t.Fatal(err)
		}
	}); allocs != 0 {
		t.Errorf("Scan([]byte) reservó %.1f veces, esperaba 0", allocs)
	}
}

// fakeWarner registra las llamadas a Printf para comprobar los avisos.
type fakeWarner struct{ keys, msgs []string }

func (f *fakeWarner) Printf(key, format string, args ...any) {
	f.keys = append(f.keys, key)
	f.msgs = append(f.msgs, fmt.Sprintf(format, args...))
}

func TestParseOrWarn(t *testing.T) {
	t.Run("válida no avisa", func(t *testing.T) {
		w := &fakeWarner{}
		m, ok := ParseOrWarn(w, "origen", "aa-bb-cc-dd-ee-ff")
		if !ok || m.String() != "AA:BB:CC:DD:EE:FF" {
			t.Fatalf("ParseOrWarn = %v, %v; want AA:BB:CC:DD:EE:FF, true", m, ok)
		}
		if len(w.keys) != 0 {
			t.Fatalf("avisos inesperados: %q", w.msgs)
		}
	})
	t.Run("vacía es silenciosa", func(t *testing.T) {
		w := &fakeWarner{}
		m, ok := ParseOrWarn(w, "origen", "")
		if ok || m.IsValid() {
			t.Fatalf("ParseOrWarn(\"\") = %v, %v; want MAC{}, false", m, ok)
		}
		if len(w.keys) != 0 {
			t.Fatalf("avisos inesperados: %q", w.msgs)
		}
	})
	t.Run("inválida avisa con la clave y el mensaje", func(t *testing.T) {
		w := &fakeWarner{}
		m, ok := ParseOrWarn(w, "origen", "xyz")
		if ok || m.IsValid() {
			t.Fatalf("ParseOrWarn(\"xyz\") = %v, %v; want MAC{}, false", m, ok)
		}
		if len(w.keys) != 1 || w.keys[0] != "origen\x00xyz" {
			t.Fatalf("claves = %q; want [\"origen\\x00xyz\"]", w.keys)
		}
		if want := `origen: ignoring invalid MAC "xyz"`; w.msgs[0] != want {
			t.Fatalf("mensaje = %q; want %q", w.msgs[0], want)
		}
	})
}
