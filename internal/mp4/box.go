package mp4

import (
	"encoding/binary"
	"errors"
)

type Box struct {
	Type  string
	Start int64
	Data  []byte
}

var ErrShort = errors.New("mp4: truncated box")

func ReadBoxes(buf []byte, base int64) ([]Box, error) {
	out := make([]Box, 0, 8)
	off := 0
	for off+8 <= len(buf) {
		size := int64(binary.BigEndian.Uint32(buf[off:]))
		typ := string(buf[off+4 : off+8])
		hdr := int64(8)
		if size == 1 {
			if off+16 > len(buf) {
				return out, ErrShort
			}
			size = int64(binary.BigEndian.Uint64(buf[off+8:]))
			hdr = 16
		}
		if size == 0 {
			size = int64(len(buf) - off)
		}
		if size < hdr || off+int(size) > len(buf) {
			return out, ErrShort
		}
		out = append(out, Box{
			Type:  typ,
			Start: base + int64(off) + hdr,
			Data:  buf[off+int(hdr) : off+int(size)],
		})
		off += int(size)
	}
	return out, nil
}

func (b Box) Children() []Box {
	kids, _ := ReadBoxes(b.Data, b.Start)
	return kids
}

func (b Box) Child(typ string) (Box, bool) {
	for _, k := range b.Children() {
		if k.Type == typ {
			return k, true
		}
	}
	return Box{}, false
}

func (b Box) Path(types ...string) (Box, bool) {
	cur := b
	for _, t := range types {
		next, ok := cur.Child(t)
		if !ok {
			return Box{}, false
		}
		cur = next
	}
	return cur, true
}

func (b Box) AllChildren(typ string) []Box {
	out := []Box{}
	for _, k := range b.Children() {
		if k.Type == typ {
			out = append(out, k)
		}
	}
	return out
}

func u8(b []byte) uint8 {
	if len(b) < 1 {
		return 0
	}
	return b[0]
}

func u16(b []byte) uint16 {
	if len(b) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}

func u32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

func u64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

func u32s(b []byte) int32 {
	return int32(u32(b))
}

func fullBoxVersion(b []byte) byte {
	return u8(b)
}

func fullBoxFlags(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(b) & 0x00FFFFFF
}
