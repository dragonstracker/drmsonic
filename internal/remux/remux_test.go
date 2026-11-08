package remux

import (
	"encoding/binary"
	"testing"
)

func lengthPrefixed(nalus ...[]byte) []byte {
	out := make([]byte, 0, 64)
	for _, n := range nalus {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(n)))
		out = append(out, l[:]...)
		out = append(out, n...)
	}
	return out
}

func TestToAnnexB(t *testing.T) {
	idr := []byte{0x65, 0x88, 0x84}
	slice := []byte{0x41, 0x9A}
	sample := lengthPrefixed(idr, slice)
	sps := [][]byte{{0x67, 0x64, 0x00}}

	out, key := ToAnnexB(sample, nil, sps, nil, false)
	if !key {
		t.Fatal("idr sample not reported as keyframe")
	}
	if len(out) < 5 || out[0] != 0 || out[3] != 1 {
		t.Fatal("missing leading start code")
	}
	if out[4] != 0x09 || out[5] != 0xF0 {
		t.Fatal("missing access unit delimiter")
	}
	if !contains(out, sps[0]) {
		t.Fatal("sps not injected on keyframe")
	}
	if !contains(out, idr) || !contains(out, slice) {
		t.Fatal("nal units missing from output")
	}
}

func TestToAnnexBNonKey(t *testing.T) {
	sample := lengthPrefixed([]byte{0x41, 0x9A})
	out, key := ToAnnexB(sample, nil, nil, nil, false)
	if key {
		t.Fatal("non idr sample reported as keyframe")
	}
	if bytesCount(out, []byte{0, 0, 0, 1}) != 2 {
		t.Fatal("expected one start code for the delimiter and one for the nal")
	}
}

func TestADTS(t *testing.T) {
	payload := make([]byte, 100)
	frame := ADTS(payload, 48000, 2)

	if frame[0] != 0xFF || frame[1] != 0xF1 {
		t.Fatal("bad sync word")
	}
	if got := int(frame[2]>>2) & 0x0F; got != 3 {
		t.Fatalf("sample rate index = %d, want 3 for 48000", got)
	}
	if got := int(frame[2]&0x01)<<2 | int(frame[3]>>6); got != 2 {
		t.Fatalf("channel configuration = %d, want 2", got)
	}
	length := int(frame[3]&0x03)<<11 | int(frame[4])<<3 | int(frame[5]>>5)
	if length != len(payload)+7 {
		t.Fatalf("frame length = %d, want %d", length, len(payload)+7)
	}
}

func contains(hay, needle []byte) bool {
	return bytesCount(hay, needle) > 0
}

func bytesCount(hay, needle []byte) int {
	n := 0
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			n++
		}
	}
	return n
}
