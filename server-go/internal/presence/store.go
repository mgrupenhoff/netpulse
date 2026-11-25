package presence

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gnacho/netpulse/agent/macaddr"
	"github.com/gnacho/netpulse/server-go/internal/db"
)

const peopleKey = "presence.people.v1"

type Person struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	MACs  []macaddr.MAC `json:"macs"`
	Color string        `json:"color"`
}

type PresenceEvent struct {
	Ts     int64       `json:"ts"`
	MAC    macaddr.MAC `json:"mac"`
	State  string      `json:"state"`
	Person string      `json:"person"`
}

type Status struct {
	Person        string `json:"person"`
	Home          bool   `json:"home"`
	LastSeen      int64  `json:"lastSeen"`
	DevicesOnline int    `json:"devicesOnline"`
}

type Store struct {
	mu          sync.RWMutex
	db          *db.DB
	people      []Person
	macToPerson map[macaddr.MAC]string
}

func NewStore(d *db.DB) *Store {
	s := &Store{
		db:          d,
		macToPerson: map[macaddr.MAC]string{},
	}
	s.load()
	return s
}

func (s *Store) load() {
	if s.db == nil {
		return
	}
	var raw string
	if err := s.db.QueryRow("SELECT value FROM kv WHERE key = ?", peopleKey).Scan(&raw); err != nil {
		return
	}
	type personDTO struct {
		ID    string   `json:"id"`
		Name  string   `json:"name"`
		MACs  []string `json:"macs"`
		Color string   `json:"color"`
	}
	var dtos []personDTO
	if json.Unmarshal([]byte(raw), &dtos) != nil {
		return
	}
	people := make([]Person, 0, len(dtos))
	for _, d := range dtos {
		macs := make([]macaddr.MAC, 0, len(d.MACs))
		for _, rawMAC := range d.MACs {
			m, err := macaddr.Parse(rawMAC)
			if err != nil {
				if rawMAC != "" {
					log.Printf("[presence] skipping invalid MAC %q for person %q: %v", rawMAC, d.ID, err)
				}
				continue
			}
			macs = append(macs, m)
		}
		people = append(people, Person{
			ID:    d.ID,
			Name:  d.Name,
			MACs:  macs,
			Color: d.Color,
		})
	}
	s.people = people
	s.rebuildIndex()
}

func (s *Store) rebuildIndex() {
	s.macToPerson = map[macaddr.MAC]string{}
	for _, p := range s.people {
		for _, mac := range p.MACs {
			if !mac.IsValid() {
				continue
			}
			s.macToPerson[mac] = p.ID
		}
	}
}

func (s *Store) save() {
	if s.db == nil {
		return
	}
	raw, _ := json.Marshal(s.people)
	_, _ = s.db.Exec(
		"INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
		peopleKey, string(raw))
}

func (s *Store) ListPeople() []Person {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Person, len(s.people))
	copy(out, s.people)
	return out
}

func (s *Store) AddPerson(p Person) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = time.Now().Format("20060102150405")
	}
	s.people = append(s.people, p)
	s.rebuildIndex()
	s.save()
	return nil
}

func (s *Store) UpdatePerson(p Person) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.people {
		if existing.ID == p.ID {
			s.people[i] = p
			s.rebuildIndex()
			s.save()
			return nil
		}
	}
	return nil
}

func (s *Store) DeletePerson(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.people {
		if p.ID == id {
			s.people = append(s.people[:i], s.people[i+1:]...)
			s.rebuildIndex()
			s.save()
			return
		}
	}
}

func (s *Store) PersonForMAC(mac macaddr.MAC) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.macToPerson[mac]
}

func (s *Store) CurrentStatus() []Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statuses := make([]Status, len(s.people))
	for i, p := range s.people {
		st := Status{Person: p.ID}
		for _, mac := range p.MACs {
			if s.db == nil {
				continue
			}
			var state string
			var ts int64
			err := s.db.QueryRow(
				"SELECT state, ts_ms FROM device_events WHERE mac = ? ORDER BY ts_ms DESC LIMIT 1",
				mac,
			).Scan(&state, &ts)
			if err == nil {
				if state == "online" {
					st.DevicesOnline++
					st.Home = true
				}
				if ts > st.LastSeen {
					st.LastSeen = ts
				}
			}
		}
		statuses[i] = st
	}
	return statuses
}

func (s *Store) Timeline(personID string, since time.Time) []PresenceEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var person *Person
	for i := range s.people {
		if s.people[i].ID == personID {
			person = &s.people[i]
			break
		}
	}
	if person == nil || s.db == nil {
		return nil
	}

	sinceMs := since.UnixMilli()
	var events []PresenceEvent
	for _, mac := range person.MACs {
		rows, err := s.db.Query(
			"SELECT ts_ms, mac, state FROM device_events WHERE mac = ? AND ts_ms >= ? ORDER BY ts_ms",
			mac, sinceMs)
		if err != nil {
			continue
		}
		for rows.Next() {
			var ev PresenceEvent
			if rows.Scan(&ev.Ts, &ev.MAC, &ev.State) == nil {
				ev.Person = personID
				events = append(events, ev)
			}
		}
		rows.Close()
	}
	return events
}
