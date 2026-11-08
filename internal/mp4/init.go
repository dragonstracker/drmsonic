package mp4

import (
	"encoding/binary"
	"strings"
)

type Track struct {
	ID        int
	Timescale int64
	Handler   string
	Codec     string
	Width     int
	Height    int
	FrameRate float64
	Rate      int
	Channels  int

	SPS         [][]byte
	PPS         [][]byte
	VPS         [][]byte
	AudioConfig []byte

	Encrypted      bool
	Scheme         string
	DefaultKID     []byte
	IVSize         int
	OriginalFormat string

	DefaultDuration uint32
	DefaultSize     uint32
	DefaultFlags    uint32
}

type Init struct {
	TrackID   int
	Timescale int64
	Tracks    []*Track
}

func ParseInit(buf []byte) (*Init, error) {
	boxes, err := ReadBoxes(buf, 0)
	if err != nil {
		return nil, err
	}
	var moov Box
	for _, b := range boxes {
		if b.Type == "moov" {
			moov = b
		}
	}
	if moov.Type == "" {
		return nil, ErrShort
	}

	trex := map[int]*trexEntry{}
	if mvex, ok := moov.Child("mvex"); ok {
		for _, t := range mvex.AllChildren("trex") {
			e := parseTrex(t.Data)
			trex[e.trackID] = e
		}
	}

	init := &Init{}
	for _, trak := range moov.AllChildren("trak") {
		tr := parseTrak(trak)
		if tr == nil {
			continue
		}
		if e, ok := trex[tr.ID]; ok {
			tr.DefaultDuration = e.duration
			tr.DefaultSize = e.size
			tr.DefaultFlags = e.flags
		}
		init.Tracks = append(init.Tracks, tr)
		if init.TrackID == 0 {
			init.TrackID = tr.ID
			init.Timescale = tr.Timescale
		}
	}
	return init, nil
}

type trexEntry struct {
	trackID  int
	duration uint32
	size     uint32
	flags    uint32
}

func parseTrex(b []byte) *trexEntry {
	if len(b) < 24 {
		return nil
	}
	return &trexEntry{
		trackID:  int(u32(b[4:])),
		duration: u32(b[12:]),
		size:     u32(b[16:]),
		flags:    u32(b[20:]),
	}
}

func parseTrak(trak Box) *Track {
	tkhd, ok := trak.Child("tkhd")
	if !ok {
		return nil
	}
	tr := &Track{}
	ver := fullBoxVersion(tkhd.Data)
	switch {
	case ver == 1 && len(tkhd.Data) >= 24:
		tr.ID = int(u32(tkhd.Data[20:]))
	case len(tkhd.Data) >= 16:
		tr.ID = int(u32(tkhd.Data[12:]))
	}

	mdia, ok := trak.Child("mdia")
	if !ok {
		return nil
	}
	if mdhd, ok := mdia.Child("mdhd"); ok {
		mv := fullBoxVersion(mdhd.Data)
		if mv == 1 && len(mdhd.Data) >= 24 {
			tr.Timescale = int64(u32(mdhd.Data[20:]))
		} else if len(mdhd.Data) >= 16 {
			tr.Timescale = int64(u32(mdhd.Data[12:]))
		}
	}
	if hdlr, ok := mdia.Child("hdlr"); ok && len(hdlr.Data) >= 12 {
		tr.Handler = string(hdlr.Data[8:12])
	}
	if tr.Timescale == 0 {
		tr.Timescale = 1000
	}

	minf, ok := mdia.Child("minf")
	if !ok {
		return tr
	}
	stbl, ok := minf.Child("stbl")
	if !ok {
		return tr
	}
	stsd, ok := stbl.Child("stsd")
	if !ok || len(stsd.Data) < 8 {
		return tr
	}
	entries, err := ReadBoxes(stsd.Data[8:], stsd.Start+8)
	if err != nil || len(entries) == 0 {
		return tr
	}
	entry := entries[0]
	tr.Codec = entry.Type
	parseSampleEntry(tr, entry)
	return tr
}

func parseSampleEntry(tr *Track, entry Box) {
	switch tr.Handler {
	case "vide":
		if len(entry.Data) >= 28 {
			tr.Width = int(u16(entry.Data[24:]))
			tr.Height = int(u16(entry.Data[26:]))
		}
		if len(entry.Data) > 78 {
			kids, _ := ReadBoxes(entry.Data[78:], entry.Start+78)
			for _, k := range kids {
				switch k.Type {
				case "avcC":
					tr.SPS, tr.PPS = parseAVCC(k.Data)
				case "hvcC":
					tr.VPS, tr.SPS, tr.PPS = parseHVCC(k.Data)
				case "sinf":
					parseSinf(tr, k)
				}
			}
		}
	case "soun":
		if len(entry.Data) >= 28 {
			tr.Channels = int(u16(entry.Data[16:]))
			tr.Rate = int(u32(entry.Data[24:]) >> 16)
		}
		if len(entry.Data) > 28 {
			base := 28
			if len(entry.Data) >= 30 {
				v := u16(entry.Data[16:])
				if v == 1 {
					base = 44
				} else if v == 2 {
					base = 64
				}
			}
			if len(entry.Data) > base {
				kids, _ := ReadBoxes(entry.Data[base:], entry.Start+int64(base))
				for _, k := range kids {
					switch k.Type {
					case "esds":
						tr.AudioConfig = parseESDS(k.Data)
					case "sinf":
						parseSinf(tr, k)
					}
				}
			}
		}
	default:
		if len(entry.Data) > 78 {
			if kids, err := ReadBoxes(entry.Data[78:], entry.Start+78); err == nil {
				for _, k := range kids {
					if k.Type == "sinf" {
						parseSinf(tr, k)
					}
				}
			}
		}
	}

	if tr.Codec == "encv" || tr.Codec == "enca" {
		tr.Encrypted = true
	}
}

func parseSinf(tr *Track, sinf Box) {
	tr.Encrypted = true
	if frma, ok := sinf.Child("frma"); ok && len(frma.Data) >= 4 {
		tr.OriginalFormat = string(frma.Data[:4])
	}
	if schm, ok := sinf.Child("schm"); ok && len(schm.Data) >= 8 {
		tr.Scheme = string(schm.Data[4:8])
	}
	if schi, ok := sinf.Child("schi"); ok {
		if tenc, ok := schi.Child("tenc"); ok {
			parseTenc(tr, tenc.Data)
		}
	}
}

func parseTenc(tr *Track, b []byte) {
	if len(b) < 20 {
		return
	}
	switch {
	case len(b) >= 24 && b[6] == 1 && (b[7] == 8 || b[7] == 16):
		tr.IVSize = int(b[7])
		tr.DefaultKID = append([]byte(nil), b[8:24]...)
	case len(b) >= 23 && b[5] == 1 && (b[6] == 8 || b[6] == 16):
		tr.IVSize = int(b[6])
		tr.DefaultKID = append([]byte(nil), b[7:23]...)
	case len(b) >= 24 && b[6] != 0 && (b[7] == 8 || b[7] == 16):
		tr.IVSize = int(b[7])
		tr.DefaultKID = append([]byte(nil), b[8:24]...)
	default:
		tr.IVSize = 8
	}
	if tr.IVSize == 0 {
		tr.IVSize = 8
	}
}

func parseAVCC(b []byte) ([][]byte, [][]byte) {
	if len(b) < 7 {
		return nil, nil
	}
	numSPS := int(b[5] & 0x1F)
	off := 6
	var sps, pps [][]byte
	for i := 0; i < numSPS && off+2 <= len(b); i++ {
		n := int(binary.BigEndian.Uint16(b[off:]))
		off += 2
		if off+n > len(b) {
			break
		}
		sps = append(sps, append([]byte(nil), b[off:off+n]...))
		off += n
	}
	if off >= len(b) {
		return sps, pps
	}
	numPPS := int(b[off])
	off++
	for i := 0; i < numPPS && off+2 <= len(b); i++ {
		n := int(binary.BigEndian.Uint16(b[off:]))
		off += 2
		if off+n > len(b) {
			break
		}
		pps = append(pps, append([]byte(nil), b[off:off+n]...))
		off += n
	}
	return sps, pps
}

func parseHVCC(b []byte) (vps, sps, pps [][]byte) {
	if len(b) < 23 {
		return
	}
	numArrays := int(b[22])
	off := 23
	for i := 0; i < numArrays && off+3 <= len(b); i++ {
		typ := b[off] & 0x3F
		count := int(binary.BigEndian.Uint16(b[off+1:]))
		off += 3
		for j := 0; j < count && off+2 <= len(b); j++ {
			n := int(binary.BigEndian.Uint16(b[off:]))
			off += 2
			if off+n > len(b) {
				return
			}
			nal := append([]byte(nil), b[off:off+n]...)
			off += n
			switch typ {
			case 32:
				vps = append(vps, nal)
			case 33:
				sps = append(sps, nal)
			case 34:
				pps = append(pps, nal)
			}
		}
	}
	return
}

func parseESDS(b []byte) []byte {
	if len(b) < 4 {
		return nil
	}
	buf := b[4:]
	off := 0
	for off+2 <= len(buf) {
		tag := buf[off]
		off++
		size := 0
		for shift := uint(0); off < len(buf); shift += 7 {
			v := buf[off]
			off++
			size |= int(v&0x7F) << shift
			if v&0x80 == 0 {
				break
			}
		}
		if off+size > len(buf) {
			return nil
		}
		if tag == 0x05 {
			return append([]byte(nil), buf[off:off+size]...)
		}
		if tag == 0x03 || tag == 0x04 {
			continue
		}
		off += size
	}
	return nil
}

func ParseAudioSpecificConfig(b []byte) (rate, channels int) {
	if len(b) < 2 {
		return 0, 0
	}
	obj := int(b[0] >> 3)
	idx := int(b[0]&0x07)<<1 | int(b[1]>>7)
	rates := []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
	rate = 0
	if idx < len(rates) {
		rate = rates[idx]
	}
	channels = int((b[1] >> 3) & 0x0F)
	_ = obj
	return
}

func SampleRateIndex(rate int) int {
	rates := []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
	for i, r := range rates {
		if r == rate {
			return i
		}
	}
	return 4
}

func IsVideoCodec(codec string) bool {
	return strings.HasPrefix(codec, "avc") || strings.HasPrefix(codec, "hvc") || strings.HasPrefix(codec, "hev") || strings.HasPrefix(codec, "vp0") || strings.HasPrefix(codec, "av01")
}
