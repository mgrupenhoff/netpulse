// Package logonce avisa una sola vez por clave, para rutas repetitivas
// (sondeos, pushes del agente, beacons) en las que el mismo problema se
// repetiría en cada tick.
//
// Cada subsistema crea su propio Logger donde le resulte cómodo, así sus
// claves y su tope no se mezclan con los de otros.
package logonce

import (
	"log"
	"sync"
	"time"
)

const (
	// defaultMax acota las claves distintas que recuerda un Logger.
	defaultMax = 64
	// defaultWindow es cada cuánto se olvida todo, para que un servidor de
	// larga duración vuelva a avisar de lo que sigue fallando.
	defaultWindow = 24 * time.Hour
)

// Logger registra cada clave una sola vez por ventana. Recuerda como mucho
// defaultMax claves; la primera que pasa el tope produce una única línea de
// supresión y las siguientes se callan hasta que la ventana se reinicia.
// Es seguro para uso concurrente: el cerrojo protege solo la contabilidad y
// la escritura del log se hace fuera de él.
type Logger struct {
	prefix string
	logf   func(string, ...any)
	now    func() time.Time // sustituible en los tests del paquete
	max    int
	window time.Duration

	mu      sync.Mutex
	seen    map[string]struct{}
	capped  bool
	started time.Time
}

// New crea un Logger que escribe con log.Printf anteponiendo prefix a cada
// línea (p. ej. "[netpulse:mac] ").
func New(prefix string) *Logger {
	return NewWithLogf(prefix, log.Printf)
}

// NewWithLogf es New con otra función de salida (útil en tests).
func NewWithLogf(prefix string, logf func(string, ...any)) *Logger {
	return &Logger{
		prefix: prefix,
		logf:   logf,
		now:    time.Now,
		max:    defaultMax,
		window: defaultWindow,
		seen:   map[string]struct{}{},
	}
}

// Printf escribe el mensaje si key no se ha visto en la ventana actual.
func (l *Logger) Printf(key, format string, args ...any) {
	switch l.remember(key) {
	case logIt:
		l.logf(l.prefix+format, args...)
	case suppress:
		l.logf("%sfurther warnings suppressed", l.prefix)
	}
}

type verdict int

const (
	skip verdict = iota
	logIt
	suppress
)

// remember anota key y decide qué hay que escribir.
func (l *Logger) remember(key string) verdict {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := l.now(); t.Sub(l.started) >= l.window {
		l.seen = map[string]struct{}{}
		l.capped = false
		l.started = t
	}
	if _, ok := l.seen[key]; ok {
		return skip
	}
	if len(l.seen) >= l.max {
		if l.capped {
			return skip
		}
		l.capped = true
		return suppress
	}
	l.seen[key] = struct{}{}
	return logIt
}
