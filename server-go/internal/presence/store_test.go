package presence

import (
	"testing"

	"github.com/gnacho/netpulse/agent/macaddr"
	"github.com/gnacho/netpulse/server-go/internal/db"
)

func TestPeopleCRUD(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer d.Close()

	s := NewStore(d)
	mac := macaddr.MustParse("aa:bb:cc:dd:ee:ff")
	if err := s.AddPerson(Person{Name: "Nacho", MACs: []macaddr.MAC{mac}, Color: "#f59e0b"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	people := s.ListPeople()
	if len(people) != 1 || people[0].Name != "Nacho" {
		t.Fatalf("list: %+v", people)
	}

	if s.PersonForMAC(mac) == "" {
		t.Fatal("MAC not indexed")
	}

	s.DeletePerson(people[0].ID)
	if len(s.ListPeople()) != 0 {
		t.Fatal("delete failed")
	}
}

func TestPersistence(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer d.Close()

	s := NewStore(d)
	_ = s.AddPerson(Person{Name: "Ana", MACs: []macaddr.MAC{macaddr.MustParse("11:22:33:44:55:66")}})

	s2 := NewStore(d)
	people := s2.ListPeople()
	if len(people) != 1 || people[0].Name != "Ana" {
		t.Fatalf("persistence: %+v", people)
	}
}

func TestCurrentStatus(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer d.Close()

	if _, err := d.Exec("INSERT INTO device_events (ts_ms, mac, state) VALUES (1000, 'AA:BB:CC:DD:EE:FF', 'online')"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	s := NewStore(d)
	_ = s.AddPerson(Person{ID: "p1", Name: "Test", MACs: []macaddr.MAC{macaddr.MustParse("aa:bb:cc:dd:ee:ff")}})

	statuses := s.CurrentStatus()
	if len(statuses) != 1 || !statuses[0].Home {
		t.Fatalf("status: %+v", statuses)
	}
}

// TestLoadSkipsInvalidMACs comprueba que un JSON legacy de presence.people.v1
// con MACs vacías o mal formadas carga la persona con sus MACs válidas y no
// indexa el valor cero MAC{}.
func TestLoadSkipsInvalidMACs(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer d.Close()

	blob := `[{"id":"p1","name":"Ana","macs":["aa:bb:cc:dd:ee:ff","","not-a-mac"],"color":"#123456"}]`
	if _, err := d.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", peopleKey, blob); err != nil {
		t.Fatalf("sembrar kv: %v", err)
	}

	s := NewStore(d)
	people := s.ListPeople()
	if len(people) != 1 {
		t.Fatalf("esperaba 1 persona cargada, obtuve %d: %+v", len(people), people)
	}
	wantMAC := macaddr.MustParse("AA:BB:CC:DD:EE:FF")
	if len(people[0].MACs) != 1 || people[0].MACs[0] != wantMAC {
		t.Fatalf("esperaba solo la MAC válida %s, obtuve %+v", wantMAC, people[0].MACs)
	}
	if s.PersonForMAC(wantMAC) != "p1" {
		t.Fatalf("la MAC válida %s no quedó indexada para p1", wantMAC)
	}
	if got := s.PersonForMAC(macaddr.MAC{}); got != "" {
		t.Fatalf("la MAC cero no debía indexarse, obtuve %q", got)
	}
}
