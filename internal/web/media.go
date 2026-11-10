package web

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"drmsonic/internal/config"
)

type Segment struct {
	Seq      uint64
	Name     string
	Duration float64
	Data     []byte
}

type SegBuffer struct {
	Seq    uint64
	Target int
	Items  []Segment
}

type mediaStore struct {
	mu     sync.RWMutex
	bufs   map[string]*SegBuffer
	access map[string]time.Time
}

func newMediaStore() *mediaStore {
	return &mediaStore{
		bufs:   make(map[string]*SegBuffer),
		access: make(map[string]time.Time),
	}
}

func mediaKey(p, s, rend string) string {
	if rend == "" {
		return p + "/" + s
	}
	return p + "/" + s + "/v/" + rend
}

func (m *mediaStore) push(p, s, rend string, data []byte, dur float64, window int) Segment {
	if window < 2 {
		window = 2
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := mediaKey(p, s, rend)
	b, ok := m.bufs[k]
	if !ok {
		b = &SegBuffer{Target: 2}
		m.bufs[k] = b
	}
	if _, seen := m.access[k]; !seen {
		m.access[k] = time.Now()
	}
	m.access[mediaKey(p, s, "")] = time.Now()
	seq := b.Seq
	seg := Segment{Seq: seq, Name: segmentName(seq), Duration: dur, Data: data}
	b.Items = append(b.Items, seg)
	keep := window + 8
	if len(b.Items) > keep {
		b.Items = b.Items[len(b.Items)-keep:]
	}
	b.Seq++
	return seg
}

func (m *mediaStore) setTarget(p, s, rend string, target int) {
	if target < 1 {
		target = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.bufs[mediaKey(p, s, rend)]; ok {
		b.Target = target
	}
}

func (m *mediaStore) find(p, s, rend, name string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.bufs[mediaKey(p, s, rend)]
	if !ok {
		return nil, false
	}
	for i := range b.Items {
		if b.Items[i].Name == name {
			return b.Items[i].Data, true
		}
	}
	return nil, false
}

func (m *mediaStore) snapshot(p, s, rend string) SegBuffer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.bufs[mediaKey(p, s, rend)]
	if !ok {
		return SegBuffer{Target: 2}
	}
	out := SegBuffer{Seq: b.Seq, Target: b.Target}
	out.Items = make([]Segment, len(b.Items))
	copy(out.Items, b.Items)
	return out
}

func (m *mediaStore) touch(p, s, rend string) {
	m.mu.Lock()
	now := time.Now()
	m.access[mediaKey(p, s, rend)] = now
	m.access[mediaKey(p, s, "")] = now
	m.mu.Unlock()
}

func (m *mediaStore) idle(p, s string) time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	at, ok := m.access[mediaKey(p, s, "")]
	if !ok {
		return 0
	}
	return time.Since(at)
}

func (m *mediaStore) drop(p, s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := p + "/" + s
	for k := range m.bufs {
		if k == prefix || strings.HasPrefix(k, prefix+"/") {
			delete(m.bufs, k)
			delete(m.access, k)
		}
	}
}

func (m *mediaStore) dump() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.bufs))
	for k, b := range m.bufs {
		first, last := uint64(0), uint64(0)
		if len(b.Items) > 0 {
			first = b.Items[0].Seq
			last = b.Items[len(b.Items)-1].Seq
		}
		out = append(out, fmt.Sprintf("%s seq=%d items=%d first=%d last=%d", k, b.Seq, len(b.Items), first, last))
	}
	return out
}

func segmentName(seq uint64) string {
	return "stream" + pad(seq) + ".ts"
}

func pad(v uint64) string {
	digits := []byte{}
	if v == 0 {
		digits = []byte{'0'}
	}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	for len(digits) < 5 {
		digits = append([]byte{'0'}, digits...)
	}
	return string(digits)
}

func (s *Server) segments(prov *config.Provider, st *config.Stream, rend string) SegBuffer {
	return s.media.snapshot(prov.Id, st.Id, rend)
}

func (s *Server) segmentData(prov *config.Provider, st *config.Stream, rend, name string) ([]byte, bool) {
	return s.media.find(prov.Id, st.Id, rend, name)
}

func windowFor(prov *config.Provider, st *config.Stream) int {
	n := st.NbAnnouncedFragments
	if n <= 0 {
		n = prov.NbAnnouncedFragments
	}
	if n <= 0 {
		n = 7
	}
	return n
}
