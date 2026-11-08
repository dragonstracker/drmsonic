package remux

import "encoding/binary"

const startCode = "\x00\x00\x00\x01"

func SplitNALs(sample []byte) [][]byte {
	out := make([][]byte, 0, 8)
	off := 0
	for off+4 <= len(sample) {
		n := int(binary.BigEndian.Uint32(sample[off:]))
		off += 4
		if n <= 0 || off+n > len(sample) {
			break
		}
		out = append(out, sample[off:off+n])
		off += n
	}
	return out
}

func ToAnnexB(sample []byte, vps, sps, pps [][]byte, hevc bool) ([]byte, bool) {
	nalus := SplitNALs(sample)
	if len(nalus) == 0 {
		return nil, false
	}

	keyframe := false
	for _, n := range nalus {
		if isKeyNal(n, hevc) {
			keyframe = true
			break
		}
	}

	size := 0
	for _, n := range nalus {
		size += 4 + len(n)
	}
	if keyframe {
		for _, p := range [][][]byte{vps, sps, pps} {
			for _, n := range p {
				size += 4 + len(n)
			}
		}
	}
	size += 6

	out := make([]byte, 0, size)
	out = append(out, startCode...)
	if hevc {
		out = append(out, 0x46, 0x01, 0x50)
	} else {
		out = append(out, 0x09, 0xF0)
	}

	if keyframe {
		for _, p := range [][][]byte{vps, sps, pps} {
			for _, n := range p {
				out = append(out, startCode...)
				out = append(out, n...)
			}
		}
	}

	for _, n := range nalus {
		out = append(out, startCode...)
		out = append(out, n...)
	}
	return out, keyframe
}

func isKeyNal(n []byte, hevc bool) bool {
	if len(n) == 0 {
		return false
	}
	if hevc {
		t := (n[0] >> 1) & 0x3F
		return t >= 16 && t <= 21
	}
	return n[0]&0x1F == 5
}

func ADTS(payload []byte, sampleRate, channels int) []byte {
	frameLen := len(payload) + 7
	hdr := make([]byte, 7)
	hdr[0] = 0xFF
	hdr[1] = 0xF1
	sf := sampleRateIndex(sampleRate)
	if channels <= 0 {
		channels = 2
	}
	hdr[2] = byte((1 << 6) | (sf << 2) | ((channels >> 2) & 1))
	hdr[3] = byte(((channels & 3) << 6) | ((frameLen >> 11) & 0x03))
	hdr[4] = byte((frameLen >> 3) & 0xFF)
	hdr[5] = byte(((frameLen & 0x07) << 5) | 0x1F)
	hdr[6] = 0xFC
	out := make([]byte, 0, frameLen)
	out = append(out, hdr...)
	out = append(out, payload...)
	return out
}

func sampleRateIndex(rate int) int {
	rates := []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
	for i, r := range rates {
		if r == rate {
			return i
		}
	}
	return 4
}
