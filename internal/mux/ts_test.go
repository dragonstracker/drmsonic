package mux

import (
	"encoding/binary"
	"testing"
)

func TestSegmentFraming(t *testing.T) {
	m := New(true, true, false)
	m.Begin()
	m.Video(90000, 87000, true, make([]byte, 700))
	m.Audio(90000, make([]byte, 300))
	seg := m.Reset()

	if len(seg) == 0 {
		t.Fatal("empty segment")
	}
	if len(seg)%packetSize != 0 {
		t.Fatalf("segment length %d is not a multiple of %d", len(seg), packetSize)
	}
	for i := 0; i < len(seg); i += packetSize {
		if seg[i] != 0x47 {
			t.Fatalf("packet %d does not start with the sync byte", i/packetSize)
		}
	}
}

func TestPATAndPMT(t *testing.T) {
	m := New(true, true, false)
	m.Begin()
	m.Video(90000, 90000, false, make([]byte, 100))
	seg := m.Reset()

	seen := map[uint16]bool{}
	for i := 0; i < len(seg); i += packetSize {
		pkt := seg[i : i+packetSize]
		pid := uint16(pkt[1]&0x1F)<<8 | uint16(pkt[2])
		seen[pid] = true
	}
	for _, pid := range []uint16{PIDPat, PIDPmt, PIDVideo} {
		if !seen[pid] {
			t.Fatalf("pid 0x%04X missing from segment", pid)
		}
	}
}

func TestTimescaleWrap(t *testing.T) {
	m := New(true, false, false)
	m.Begin()
	m.Video(1<<33-1, 1<<33-1, false, make([]byte, 64))
	seg := m.Reset()

	if len(seg)%packetSize != 0 {
		t.Fatalf("segment length %d is not a multiple of %d", len(seg), packetSize)
	}
	var pts uint64
	for i := 0; i < len(seg); i += packetSize {
		pkt := seg[i : i+packetSize]
		pid := uint16(pkt[1]&0x1F)<<8 | uint16(pkt[2])
		if pid != PIDVideo {
			continue
		}
		if pkt[1]&0x40 == 0 {
			continue
		}
		off := 4
		if pkt[3]&0x20 != 0 {
			off += 1 + int(pkt[4])
		}
		hdr := pkt[off:]
		if len(hdr) < 14 || hdr[0] != 0 || hdr[1] != 0 || hdr[2] != 1 {
			t.Fatal("missing PES start code")
		}
		pts = readTimestamp(hdr[9:14])
		break
	}
	if pts != 1<<33-1 {
		t.Fatalf("pts round trip mismatch: got %d", pts)
	}
}

func readTimestamp(b []byte) uint64 {
	v := uint64(b[0]&0x0E) << 29
	v |= uint64(binary.BigEndian.Uint16(b[1:3])>>1) << 15
	v |= uint64(binary.BigEndian.Uint16(b[3:5]) >> 1)
	return v
}
