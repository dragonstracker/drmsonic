package web

import (
	"sync"

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
	mu   sync.RWMutex
	bufs map[string]*SegBuffer
}

func newMediaStore() *mediaStore {
	return &mediaStore{bufs: make(map[string]*SegBuffer)}
}

func mediaKey(p, s string) string {
	return p + "/" + s
}

func (m *mediaStore) push(p, s string, data []byte, dur float64, window int) Segment {
	if window < 2 {
		window = 2
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := mediaKey(p, s)
	b, ok := m.bufs[k]
	if !ok {
		b = &SegBuffer{Target: 2}
		m.bufs[k] = b
	}
	seq := b.Seq
	seg := Segment{Seq: seq, Name: segmentName(seq), Duration: dur, Data: data}
	b.Items = append(b.Items, seg)
	if len(b.Items) > window {
		b.Items = b.Items[len(b.Items)-window:]
	}
	b.Seq++
	return seg
}

func (m *mediaStore) setTarget(p, s string, target int) {
	if target < 1 {
		target = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.bufs[mediaKey(p, s)]; ok {
		b.Target = target
	}
}

func (m *mediaStore) find(p, s, name string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.bufs[mediaKey(p, s)]
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

func (m *mediaStore) snapshot(p, s string) SegBuffer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.bufs[mediaKey(p, s)]
	if !ok {
		return SegBuffer{Target: 2}
	}
	out := SegBuffer{Seq: b.Seq, Target: b.Target}
	out.Items = make([]Segment, len(b.Items))
	copy(out.Items, b.Items)
	return out
}

func (m *mediaStore) drop(p, s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bufs, mediaKey(p, s))
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

func (s *Server) segments(prov *config.Provider, st *config.Stream) SegBuffer {
	return s.media.snapshot(prov.Id, st.Id)
}

func (s *Server) segmentData(prov *config.Provider, st *config.Stream, name string) ([]byte, bool) {
	return s.media.find(prov.Id, st.Id, name)
}

func windowFor(prov *config.Provider, st *config.Stream) int {
	n := st.NbAnnouncedFragments
	if n <= 0 {
		n = prov.NbAnnouncedFragments
	}
	if n <= 0 {
		n = 6
	}
	return n
}
