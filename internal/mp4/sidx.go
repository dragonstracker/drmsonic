package mp4

import "errors"

type SidxRef struct {
	Size     int64
	Duration int64
}

type Sidx struct {
	Timescale   int64
	Earliest    int64
	FirstOffset int64
	End         int64
	Refs        []SidxRef
}

func ParseSidx(buf []byte, base int64) (*Sidx, error) {
	boxes, _ := ReadBoxes(buf, base)
	for _, b := range boxes {
		if b.Type != "sidx" {
			continue
		}
		d := b.Data
		if len(d) < 12 {
			return nil, ErrShort
		}
		out := &Sidx{
			Timescale:   int64(u32(d[8:])),
			End:         b.Start + int64(len(d)),
			FirstOffset: 0,
		}
		off := 0
		if fullBoxVersion(d) == 0 {
			if len(d) < 20 {
				return nil, ErrShort
			}
			out.Earliest = int64(u32(d[12:]))
			out.FirstOffset = int64(u32(d[16:]))
			off = 20
		} else {
			if len(d) < 28 {
				return nil, ErrShort
			}
			out.Earliest = int64(u64(d[12:]))
			out.FirstOffset = int64(u64(d[20:]))
			off = 28
		}
		if len(d) < off+4 {
			return nil, ErrShort
		}
		count := int(u16(d[off:]))
		off += 4
		if out.Timescale <= 0 {
			out.Timescale = 1000
		}
		for i := 0; i < count && off+12 <= len(d); i++ {
			word := u32(d[off:])
			size := int64(word & 0x7FFFFFFF)
			dur := int64(u32(d[off+4:]))
			off += 12
			if word>>31 == 1 {
				continue
			}
			out.Refs = append(out.Refs, SidxRef{Size: size, Duration: dur})
		}
		return out, nil
	}
	return nil, errors.New("mp4: no sidx")
}
