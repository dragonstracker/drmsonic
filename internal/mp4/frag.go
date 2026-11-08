package mp4

import "errors"

var ErrNoMoof = errors.New("mp4: no moof")

type Subsample struct {
	Clear     uint16
	Protected uint32
}

type Sample struct {
	Offset     int
	Size       int
	Duration   uint32
	CTS        int32
	Flags      uint32
	IV         []byte
	Subsamples []Subsample
}

type Fragment struct {
	TrackID  int
	Sequence uint32
	BaseTime uint64
	Length   uint64
	Samples  []Sample
	DataAt   int
	Buf      []byte
}

func ParseFragment(buf []byte, ivSize int) (*Fragment, error) {
	boxes, err := ReadBoxes(buf, 0)
	if err != nil {
		return nil, err
	}

	var moof, mdat Box
	for _, b := range boxes {
		switch b.Type {
		case "moof":
			moof = b
		case "mdat":
			if mdat.Type == "" {
				mdat = b
			}
		}
	}
	if moof.Type == "" {
		return nil, ErrNoMoof
	}
	moofStart := moof.Start - 8

	f := &Fragment{Buf: buf, DataAt: int(mdat.Start)}
	if mfhd, ok := moof.Child("mfhd"); ok && len(mfhd.Data) >= 8 {
		f.Sequence = u32(mfhd.Data[4:])
	}

	trafs := moof.AllChildren("traf")
	if len(trafs) == 0 {
		return nil, ErrNoMoof
	}
	traf := trafs[0]

	tfhd, ok := traf.Child("tfhd")
	if !ok || len(tfhd.Data) < 8 {
		return nil, ErrShort
	}
	flags := fullBoxFlags(tfhd.Data)
	f.TrackID = int(u32(tfhd.Data[4:]))

	pos := 8
	base := moofStart
	defDuration := uint32(0)
	defSize := uint32(0)
	defFlags := uint32(0)

	if flags&0x000001 != 0 {
		if pos+8 > len(tfhd.Data) {
			return nil, ErrShort
		}
		base = int64(u64(tfhd.Data[pos:]))
		pos += 8
	} else if flags&0x020000 != 0 {
		base = moofStart
	}
	if flags&0x000002 != 0 {
		pos += 4
	}
	if flags&0x000008 != 0 {
		defDuration = u32(tfhd.Data[pos:])
		pos += 4
	}
	if flags&0x000010 != 0 {
		defSize = u32(tfhd.Data[pos:])
		pos += 4
	}
	if flags&0x000020 != 0 {
		defFlags = u32(tfhd.Data[pos:])
		pos += 4
	}

	if tfdt, ok := traf.Child("tfdt"); ok {
		if fullBoxVersion(tfdt.Data) == 1 {
			f.BaseTime = u64(tfdt.Data[4:])
		} else {
			f.BaseTime = uint64(u32(tfdt.Data[4:]))
		}
	}

	senc, hasSenc := traf.Child("senc")
	var sencData []byte
	var sencFlags uint32
	if hasSenc {
		sencData = senc.Data
		sencFlags = fullBoxFlags(sencData)
	}

	cursor := int(base)
	time := uint64(0)
	var sencPos int
	if hasSenc && ivSize == 0 {
		ivSize = 8
	}
	if hasSenc {
		sencPos = 8
	}

	for _, trun := range traf.AllChildren("trun") {
		if len(trun.Data) < 8 {
			return nil, ErrShort
		}
		tf := fullBoxFlags(trun.Data)
		count := int(u32(trun.Data[4:]))
		p := 8
		if tf&0x000001 != 0 {
			cursor = int(base) + int(u32s(trun.Data[p:]))
			p += 4
		}
		if tf&0x000004 != 0 {
			p += 4
		}

		for i := 0; i < count; i++ {
			s := Sample{
				Offset:   cursor,
				Duration: defDuration,
				Flags:    defFlags,
			}
			if tf&0x000100 != 0 {
				s.Duration = u32(trun.Data[p:])
				p += 4
			}
			s.Size = int(defSize)
			if tf&0x000200 != 0 {
				s.Size = int(u32(trun.Data[p:]))
				p += 4
			}
			if tf&0x000400 != 0 {
				s.Flags = u32(trun.Data[p:])
				p += 4
			}
			if tf&0x000800 != 0 {
				if fullBoxVersion(trun.Data) == 0 {
					s.CTS = int32(u32(trun.Data[p:]))
				} else {
					s.CTS = u32s(trun.Data[p:])
				}
				p += 4
			}
			if s.Duration == 0 {
				s.Duration = 1
			}
			time += uint64(s.Duration)

			if hasSenc {
				if ivSize > 0 && sencPos+ivSize <= len(sencData) {
					s.IV = append([]byte(nil), sencData[sencPos:sencPos+ivSize]...)
					sencPos += ivSize
				}
				if sencFlags&0x000002 != 0 && sencPos+2 <= len(sencData) {
					n := int(u16(sencData[sencPos:]))
					sencPos += 2
					for j := 0; j < n && sencPos+6 <= len(sencData); j++ {
						s.Subsamples = append(s.Subsamples, Subsample{
							Clear:     u16(sencData[sencPos:]),
							Protected: u32(sencData[sencPos+2:]),
						})
						sencPos += 6
					}
				}
			}

			cursor += s.Size
			f.Samples = append(f.Samples, s)
		}
	}

	f.Length = time
	return f, nil
}

func (f *Fragment) Data(s Sample) ([]byte, bool) {
	start := s.Offset
	if start < 0 || s.Size <= 0 || start+s.Size > len(f.Buf) {
		return nil, false
	}
	return f.Buf[start : start+s.Size], true
}

func (f *Fragment) Keyframe() bool {
	for _, s := range f.Samples {
		if s.Flags&0x00010000 == 0 {
			return true
		}
		return false
	}
	return false
}
