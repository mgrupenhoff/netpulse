package logonce

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// capture devuelve un Logger con salida y reloj controlados por el test.
func capture() (l *Logger, lines func() []string, clock *time.Time) {
	var mu sync.Mutex
	var got []string
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock = &t0
	l = NewWithLogf("[test] ", func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, fmt.Sprintf(format, args...))
	})
	l.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return *clock }
	return l, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), got...) }, clock
}

func TestPrintfOncePerKey(t *testing.T) {
	l, lines, _ := capture()
	l.Printf("a", "aviso %d", 1)
	l.Printf("a", "aviso %d", 2)
	l.Printf("b", "otro")
	got := lines()
	if len(got) != 2 || got[0] != "[test] aviso 1" || got[1] != "[test] otro" {
		t.Fatalf("líneas = %q", got)
	}
}

func TestCapLogsSuppressionOnce(t *testing.T) {
	l, lines, _ := capture()
	for i := 0; i < defaultMax+10; i++ {
		l.Printf(fmt.Sprint(i), "clave %d", i)
	}
	got := lines()
	if len(got) != defaultMax+1 {
		t.Fatalf("líneas = %d, esperaba %d avisos y una de supresión", len(got), defaultMax+1)
	}
	if last := got[len(got)-1]; last != "[test] further warnings suppressed" {
		t.Errorf("última línea = %q", last)
	}
}

func TestLoggersAreIndependent(t *testing.T) {
	ruidoso, _, _ := capture()
	tranquilo, lines, _ := capture()
	for i := 0; i < defaultMax+10; i++ {
		ruidoso.Printf(fmt.Sprint(i), "x")
	}
	tranquilo.Printf("k", "sigo avisando")
	if got := lines(); len(got) != 1 {
		t.Fatalf("un Logger lleno no debe callar a otro: %q", got)
	}
}

func TestResetAfterWindow(t *testing.T) {
	l, lines, clock := capture()
	for i := 0; i < defaultMax+1; i++ {
		l.Printf(fmt.Sprint(i), "x")
	}
	l.Printf("0", "x")
	before := len(lines())
	*clock = clock.Add(defaultWindow + time.Minute)
	l.Printf("0", "x")
	l.Printf("nueva", "x")
	if got := len(lines()) - before; got != 2 {
		t.Fatalf("tras la ventana: %d líneas nuevas, esperaba 2 (olvida claves y tope)", got)
	}
}

func TestConcurrent(t *testing.T) {
	l, _, _ := capture()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				l.Printf(fmt.Sprint((i+g)%100), "x %d", i)
			}
		}(g)
	}
	wg.Wait()
}
