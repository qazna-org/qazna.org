package stream

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"math/rand"
	"sync"
	"time"
)

// Location represents an approximate geographical point used for visualisation.
type Location struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// TransferEvent describes a money transfer between two regions for the map stream.
type TransferEvent struct {
	From      Location  `json:"from"`
	To        Location  `json:"to"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	Timestamp time.Time `json:"timestamp"`
}

// Stream fan-outs transfer events to all active subscribers (SSE/WebSocket clients).
type Stream struct {
	mu        sync.RWMutex
	subs      map[int]chan TransferEvent
	next      int
	rnd       *rand.Rand
	locations []Location
}

// New initialises an empty stream with a set of demo locations.
func New() *Stream {
	return &Stream{
		subs: make(map[int]chan TransferEvent),
		rnd:  rand.New(rand.NewSource(time.Now().UnixNano())),
		locations: []Location{
			{Name: "New York", Lat: 40.7128, Lon: -74.0060},
			{Name: "London", Lat: 51.5072, Lon: -0.1276},
			{Name: "Zurich", Lat: 47.3769, Lon: 8.5417},
			{Name: "Singapore", Lat: 1.3521, Lon: 103.8198},
			{Name: "Tokyo", Lat: 35.6762, Lon: 139.6503},
			{Name: "Hong Kong", Lat: 22.3193, Lon: 114.1694},
			{Name: "Dubai", Lat: 25.2048, Lon: 55.2708},
			{Name: "Frankfurt", Lat: 50.1109, Lon: 8.6821},
			{Name: "Sydney", Lat: -33.8688, Lon: 151.2093},
			{Name: "Toronto", Lat: 43.6532, Lon: -79.3832},
			{Name: "São Paulo", Lat: -23.5558, Lon: -46.6396},
			{Name: "Johannesburg", Lat: -26.2041, Lon: 28.0473},
			{Name: "Almaty", Lat: 43.2389, Lon: 76.8897},
			{Name: "Astana", Lat: 51.1694, Lon: 71.4491},
			{Name: "Beijing", Lat: 39.9042, Lon: 116.4074},
		},
	}
}

// Subscribe registers a subscriber and returns a channel which will receive events.
// The channel is closed when the provided context ends.
func (s *Stream) Subscribe(ctx context.Context) <-chan TransferEvent {
	ch := make(chan TransferEvent, 16)

	s.mu.Lock()
	id := s.next
	s.next++
	s.subs[id] = ch
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.subs, id)
		close(ch)
		s.mu.Unlock()
	}()

	return ch
}

// Publish fan-outs the event to all subscribers.
func (s *Stream) Publish(evt TransferEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.subs {
		select {
		case ch <- evt:
		default:
			// Drop when subscriber is slow to avoid blocking.
		}
	}
}

// LocationForID deterministically maps an account identifier to one of demo locations.
func (s *Stream) LocationForID(id string) Location {
	if len(s.locations) == 0 {
		return Location{}
	}
	hash := sha1.Sum([]byte(id))
	val := binary.BigEndian.Uint32(hash[:4])
	idx := int(val % uint32(len(s.locations)))
	return s.locations[idx]
}

// RandomDemoEvent creates an artificial flow for demo purposes.
func (s *Stream) RandomDemoEvent() TransferEvent {
	if len(s.locations) < 2 {
		return TransferEvent{Timestamp: time.Now().UTC()}
	}
	fromIdx := s.rnd.Intn(len(s.locations))
	toIdx := s.rnd.Intn(len(s.locations) - 1)
	if toIdx >= fromIdx {
		toIdx++
	}
	amount := int64(1000 + s.rnd.Intn(1_000_000)) // minor units
	return TransferEvent{
		From:      s.locations[fromIdx],
		To:        s.locations[toIdx],
		Amount:    amount,
		Currency:  "QZN",
		Timestamp: time.Now().UTC(),
	}
}

// StartDemo emits random events at the provided interval until the returned stop
// function is called.
func (s *Stream) StartDemo(interval time.Duration) func() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Publish(s.RandomDemoEvent())
			}
		}
	}()
	return cancel
}
