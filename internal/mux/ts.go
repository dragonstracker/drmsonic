package mux

const (
	PIDPat   = 0x0000
	PIDPmt   = 0x1000
	PIDVideo = 0x0100
	PIDAudio = 0x0101

	streamH264 = 0x1B
	streamH265 = 0x24
	streamAAC  = 0x0F

	packetSize = 188
	payloadMax = 184
)

type Muxer struct {
	buf   []byte
	cc    map[uint16]byte
	video bool
	audio bool
	hevc  bool
	lang  [3]byte
}

func New(video, audio, hevc bool) *Muxer {
	return &Muxer{
		cc:    make(map[uint16]byte),
		video: video,
		audio: audio,
		hevc:  hevc,
		lang:  [3]byte{'e', 'n', 'g'},
	}
}

func (m *Muxer) SetLanguage(lang string) {
	if len(lang) == 3 {
		copy(m.lang[:], lang)
	}
}

func (m *Muxer) Begin() {
	m.buf = append(m.buf, m.psi(PIDPat, m.pat())...)
	m.buf = append(m.buf, m.psi(PIDPmt, m.pmt())...)
}

func (m *Muxer) Reset() []byte {
	out := m.buf
	m.buf = nil
	return out
}

func (m *Muxer) Video(pts, dts int64, key bool, es []byte) {
	if len(es) == 0 {
		return
	}
	if dts > pts {
		dts = pts
	}
	m.writePES(PIDVideo, 0xE0, pts, dts, es, key)
}

func (m *Muxer) Audio(pts int64, es []byte) {
	if len(es) == 0 {
		return
	}
	m.writePES(PIDAudio, 0xC0, pts, pts, es, false)
}

func (m *Muxer) pesHeader(streamID byte, pts, dts int64) []byte {
	withDTS := dts != pts
	hdr := make([]byte, 0, 19)
	hdr = append(hdr, 0x00, 0x00, 0x01, streamID, 0x00, 0x00, 0x80)
	if withDTS {
		hdr = append(hdr, 0xC0, 10)
	} else {
		hdr = append(hdr, 0x80, 5)
	}
	hdr = appendPTS(hdr, pts, 0x02)
	if withDTS {
		hdr = appendPTS(hdr, dts, 0x01)
	}
	return hdr
}

func (m *Muxer) writePES(pid uint16, streamID byte, pts, dts int64, es []byte, key bool) {
	header := m.pesHeader(streamID, pts, dts)

	first := true
	remaining := es
	for len(remaining) > 0 {
		limit := payloadMax - len(header)
		pcr := false
		if first && key {
			limit -= 8
			pcr = true
		}
		if limit <= 0 {
			limit = 1
		}
		n := limit
		if n > len(remaining) {
			n = len(remaining)
		}

		if first {
			chunk := make([]byte, 0, len(header)+n)
			chunk = append(chunk, header...)
			chunk = append(chunk, remaining[:n]...)
			remaining = remaining[n:]
			m.packet(pid, true, pcr, dts, chunk)
			first = false
			continue
		}
		m.packet(pid, false, false, 0, remaining[:n])
		remaining = remaining[n:]
	}
}

func (m *Muxer) packet(pid uint16, pusi, pcr bool, pcrValue int64, payload []byte) {
	n := len(payload)
	if n > payloadMax {
		n = payloadMax
		payload = payload[:n]
	}

	afLen := -1
	if pcr || n < payloadMax {
		afLen = payloadMax - n - 1
	}
	if pcr && afLen < 7 {
		afLen = 7
	}

	cc := m.cc[pid]
	flags := byte(0x10) | (cc & 0x0F)
	if pusi {
		flags |= 0x40
	}
	if afLen >= 0 {
		flags |= 0x20
	}
	m.cc[pid] = (cc + 1) & 0x0F

	m.buf = append(m.buf,
		0x47,
		byte(pid>>8)|(flags&0x40),
		byte(pid),
		flags&0x3F,
	)

	if afLen >= 0 {
		m.buf = append(m.buf, byte(afLen))
		if afLen > 0 {
			var afFlags byte
			if pcr {
				afFlags = 0x10
			}
			m.buf = append(m.buf, afFlags)
			used := 1
			if pcr {
				m.buf = appendPCR(m.buf, pcrValue)
				used += 6
			}
			for used < afLen {
				m.buf = append(m.buf, 0xFF)
				used++
			}
		}
	}
	m.buf = append(m.buf, payload...)
}

func (m *Muxer) pat() []byte {
	section := []byte{
		0x00, 0xB0, 0x00,
		0x00, 0x01, 0xC1, 0x00, 0x00,
		0x00, 0x01,
		0xE0 | byte(PIDPmt>>8), byte(PIDPmt & 0xFF),
	}
	section[2] = byte(len(section) - 3 + 4)
	return append(section, crc32mpeg(section)...)
}

func (m *Muxer) pmt() []byte {
	es := []byte{}
	if m.video {
		t := byte(streamH264)
		if m.hevc {
			t = streamH265
		}
		es = append(es, t, byte(PIDVideo>>8), byte(PIDVideo&0xFF), 0xF0, 0x00)
	}
	if m.audio {
		es = append(es,
			streamAAC, byte(PIDAudio>>8), byte(PIDAudio&0xFF), 0xF0, 0x06,
			0x0A, 0x04, m.lang[0], m.lang[1], m.lang[2], 0x00,
		)
	}

	pcrPID := PIDVideo
	if !m.video {
		pcrPID = PIDAudio
	}

	section := []byte{
		0x02, 0xB0, 0x00,
		0x00, 0x01, 0xC1, 0x00, 0x00,
		byte(pcrPID >> 8), byte(pcrPID & 0xFF),
		0xF0, 0x00,
	}
	section = append(section, es...)
	section[2] = byte(len(section) - 3 + 4)
	return append(section, crc32mpeg(section)...)
}

func (m *Muxer) psi(pid uint16, section []byte) []byte {
	cc := m.cc[pid]
	m.cc[pid] = (cc + 1) & 0x0F

	out := make([]byte, 0, packetSize)
	out = append(out, 0x47, byte(pid>>8)|0x40, byte(pid), 0x10|(cc&0x0F), 0x00)
	out = append(out, section...)
	for len(out) < packetSize {
		out = append(out, 0xFF)
	}
	return out
}

func appendPTS(dst []byte, ts int64, prefix byte) []byte {
	ts &= (1 << 33) - 1
	return append(dst,
		byte(uint64(prefix)<<4|((uint64(ts)>>30)&0x07)<<1|1),
		byte(ts>>22),
		byte((((ts>>15)&0x7F)<<1)|1),
		byte(ts>>7),
		byte(((ts&0x7F)<<1)|1),
	)
}

func appendPCR(dst []byte, pcr int64) []byte {
	if pcr < 0 {
		pcr = 0
	}
	base := pcr
	return append(dst,
		byte(base>>25),
		byte(base>>17),
		byte(base>>9),
		byte(base>>1),
		byte(((base&1)<<7)|0x7E),
		0x00,
	)
}

var crcTable = func() [256]uint32 {
	var t [256]uint32
	for i := 0; i < 256; i++ {
		c := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if c&0x80000000 != 0 {
				c = (c << 1) ^ 0x04C11DB7
			} else {
				c <<= 1
			}
		}
		t[i] = c
	}
	return t
}()

func crc32mpeg(data []byte) []byte {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc = (crc << 8) ^ crcTable[byte(crc>>24)^b]
	}
	return []byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)}
}
