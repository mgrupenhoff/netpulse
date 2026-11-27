package macaddr

import (
	"bytes"
	"database/sql/driver"
	"encoding"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ErrSyntax indica que una cadena no representa una dirección MAC válida.
var ErrSyntax = errors.New("macaddr: invalid MAC syntax")

const (
	hexUpper = "0123456789ABCDEF"
	hexLower = "0123456789abcdef"
)

var (
	_ encoding.TextAppender    = MAC{}
	_ encoding.TextMarshaler   = MAC{}
	_ encoding.TextUnmarshaler = (*MAC)(nil)
	_ driver.Valuer            = MAC{}
)

// MAC representa una dirección Ethernet de 6 octetos (EUI-48) en forma
// canónica (AA:BB:CC:DD:EE:FF al formatearse). A diferencia de
// net.HardwareAddr / net.ParseMAC, es un tipo valor comparable de tamaño fijo
// y solo acepta Ethernet de 6 octetos (no EUI-64 de 8 octetos, IPoIB de 20
// octetos ni notación Cisco con puntos como aabb.ccdd.eeff, coherente con
// Normalize). Su valor cero es inválido/ausente (IsValid == false), al estilo
// de net/netip.Addr: fuera de este paquete solo se construye con From6, Parse,
// ParseSeparated, ParseColon, MustParse, UnmarshalText o Scan. No conserva
// formato original para que == y map[MAC]T dependan únicamente de los 6
// octetos y de valid.
type MAC struct {
	b     [6]byte
	valid bool
}

// From6 construye una MAC válida a partir de sus 6 octetos (p. ej. desde PDU SNMP).
func From6(b [6]byte) MAC { return MAC{b: b, valid: true} }

// Parse valida y normaliza raw a una MAC. Acepta ':' o '-' uniformes y 12
// dígitos hex sin separador, ignorando espacios alrededor.
func Parse(raw string) (MAC, error) {
	b, ok := parseRaw(strings.TrimSpace(raw))
	if !ok {
		return MAC{}, fmt.Errorf("%w %q", ErrSyntax, raw)
	}
	return MAC{b: b, valid: true}, nil
}

// Warner lo cumple *logonce.Logger de server-go (avisa una vez por clave).
type Warner interface {
	Printf(key, format string, args ...any)
}

// ParseOrWarn es Parse para entrada externa (sondeos, pushes, beacons):
// "" -> (MAC{}, false) sin aviso; un valor inválido -> (MAC{}, false) y
// w.Printf una vez por (what, raw).
func ParseOrWarn(w Warner, what, raw string) (MAC, bool) {
	if raw == "" {
		return MAC{}, false
	}
	m, err := Parse(raw)
	if err != nil {
		w.Printf(what+"\x00"+raw, "%s: ignoring invalid MAC %q", what, raw)
		return MAC{}, false
	}
	return m, true
}

// ParseSeparated es Parse pero exige separador (':' o '-'), descartando 12
// dígitos hex seguidos (p. ej. para distinguir slugs o hostnames).
func ParseSeparated(raw string) (MAC, error) {
	if len(strings.TrimSpace(raw)) != 17 {
		return MAC{}, fmt.Errorf("%w %q", ErrSyntax, raw)
	}
	return Parse(raw)
}

// ParseColon exige la forma con ':' (AA:BB:CC:DD:EE:FF en cualquier caja).
// Sirve para campos mixtos como topology_overrides donde un id de router puede
// parecer una MAC con guiones o 12 dígitos hex.
func ParseColon(raw string) (MAC, error) {
	s := strings.TrimSpace(raw)
	if len(s) != 17 || strings.Count(s, ":") != 5 {
		return MAC{}, fmt.Errorf("%w (colon-separated required) %q", ErrSyntax, raw)
	}
	return Parse(s)
}

// MustParse llama a Parse y entra en pánico si raw no es una MAC válida.
// Pensado para constantes, fixtures y tests (como netip.MustParseAddr).
func MustParse(raw string) MAC {
	m, err := Parse(raw)
	if err != nil {
		panic(err)
	}
	return m
}

// IsValid informa si m contiene una MAC (false solo para el valor cero).
func (m MAC) IsValid() bool { return m.valid }

// As6 devuelve los 6 octetos de la MAC (todos cero si !IsValid()).
func (m MAC) As6() [6]byte { return m.b }

// Compare ordena dos MAC de forma determinista: el valor cero va antes que
// cualquier MAC válida, y entre válidas coincide con el orden lexicográfico
// de String().
func (m MAC) Compare(other MAC) int {
	if m.valid != other.valid {
		if !m.valid {
			return -1
		}
		return 1
	}
	return bytes.Compare(m.b[:], other.b[:])
}

// IsLocallyAdministered informa si tiene activo el bit de administración local
// (bit 1 del primer octeto: MACs aleatorias de móviles, VMs, etc.).
func (m MAC) IsLocallyAdministered() bool { return m.valid && m.b[0]&0x02 != 0 }

// AppendText implementa encoding.TextAppender: escribe AA:BB:CC:DD:EE:FF al
// final de dst sin reservas intermedias (usado por encoding/json en Go 1.24+).
func (m MAC) AppendText(dst []byte) ([]byte, error) {
	if !m.valid {
		return dst, nil
	}
	var buf [17]byte
	formatMAC(&buf, m.b, hexUpper, ':')
	return append(dst, buf[:]...), nil
}

// MarshalText implementa encoding.TextMarshaler (campos y claves map[MAC]T en JSON).
func (m MAC) MarshalText() ([]byte, error) {
	if !m.valid {
		return nil, nil
	}
	return m.AppendText(make([]byte, 0, 17))
}

// UnmarshalText parsea una MAC desde texto/JSON. La cadena vacía produce el
// valor cero sin error; cualquier texto no vacío inválido devuelve error.
func (m *MAC) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*m = MAC{}
		return nil
	}
	parsed, err := parseBytes(b)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// String devuelve la MAC canónica ("AA:BB:CC:DD:EE:FF") o "" si es el valor cero.
func (m MAC) String() string {
	if !m.valid {
		return ""
	}
	var buf [17]byte
	formatMAC(&buf, m.b, hexUpper, ':')
	return string(buf[:])
}

// Lower devuelve la MAC en minúsculas con ':' (aa:bb:cc:dd:ee:ff), para la
// frontera UCI de OpenWrt (reservas DHCP, reglas de bloqueo).
func (m MAC) Lower() string {
	if !m.valid {
		return ""
	}
	var buf [17]byte
	formatMAC(&buf, m.b, hexLower, ':')
	return string(buf[:])
}

// DeviceID devuelve el identificador de Device (aa-bb-cc-dd-ee-ff) o "" si
// m es el valor cero.
func (m MAC) DeviceID() string {
	if !m.valid {
		return ""
	}
	var buf [17]byte
	formatMAC(&buf, m.b, hexLower, '-')
	return string(buf[:])
}

// Scan implementa sql.Scanner. NULL y "" producen el valor cero; una cadena no
// vacía se valida y normaliza con Parse.
func (m *MAC) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*m = MAC{}
		return nil
	case string:
		if v == "" {
			*m = MAC{}
			return nil
		}
		parsed, err := Parse(v)
		if err != nil {
			return err
		}
		*m = parsed
		return nil
	case []byte:
		if len(v) == 0 {
			*m = MAC{}
			return nil
		}
		parsed, err := parseBytes(v)
		if err != nil {
			return err
		}
		*m = parsed
		return nil
	default:
		return fmt.Errorf("macaddr: unsupported SQL type %T", src)
	}
}

// Value implementa driver.Valuer. Siempre escribe el texto canónico (o "").
func (m MAC) Value() (driver.Value, error) { return m.String(), nil }

func formatMAC(buf *[17]byte, b [6]byte, digits string, sep byte) {
	for i, v := range b {
		buf[i*3] = digits[v>>4]
		buf[i*3+1] = digits[v&0x0f]
		if i < 5 {
			buf[i*3+2] = sep
		}
	}
}

func parseBytes(b []byte) (MAC, error) {
	octets, ok := parseRaw(bytes.TrimSpace(b))
	if !ok {
		return MAC{}, fmt.Errorf("%w %q", ErrSyntax, string(b))
	}
	return MAC{b: octets, valid: true}, nil
}

// parseRaw valida e indexa byte a byte tanto string como []byte ya recortados.
// Es seguro con texto UTF-8: todos los caracteres válidos de una MAC (0-9,
// a-f, A-F, ':' y '-') son ASCII de 7 bits (< 0x80), y en UTF-8 cualquier byte
// de un carácter multibyte es >= 0x80, así que nunca puede confundirse con un
// dígito hex ni con un separador.
func parseRaw[T ~string | ~[]byte](s T) ([6]byte, bool) {
	var out [6]byte
	var hexBuf [12]byte
	switch len(s) {
	case 17:
		sep := s[2]
		if sep != ':' && sep != '-' {
			return out, false
		}
		for i := 0; i < 6; i++ {
			if i > 0 && s[i*3-1] != sep {
				return out, false
			}
			hexBuf[i*2] = s[i*3]
			hexBuf[i*2+1] = s[i*3+1]
		}
	case 12:
		copy(hexBuf[:], s)
	default:
		return out, false
	}
	_, err := hex.Decode(out[:], hexBuf[:])
	return out, err == nil
}
