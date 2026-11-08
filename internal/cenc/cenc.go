package cenc

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"

	"drmsonic/internal/mp4"
)

var ErrNoKey = errors.New("cenc: no key for sample")

func DecryptSample(key, iv, sample []byte, subs []mp4.Subsample) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	counter := make([]byte, 16)
	copy(counter, iv)

	stream := cipher.NewCTR(block, counter)

	if len(subs) == 0 {
		out := make([]byte, len(sample))
		stream.XORKeyStream(out, sample)
		return out, nil
	}

	out := make([]byte, 0, len(sample))
	pos := 0
	for _, sub := range subs {
		clear := int(sub.Clear)
		prot := int(sub.Protected)
		if pos+clear > len(sample) {
			break
		}
		out = append(out, sample[pos:pos+clear]...)
		pos += clear
		if pos+prot > len(sample) {
			prot = len(sample) - pos
		}
		if prot > 0 {
			part := make([]byte, prot)
			stream.XORKeyStream(part, sample[pos:pos+prot])
			out = append(out, part...)
			pos += prot
		}
	}
	if pos < len(sample) {
		out = append(out, sample[pos:]...)
	}
	return out, nil
}

func KeyFor(keys map[string][]byte, kid []byte, iv []byte) []byte {
	if kid != nil {
		if k, ok := keys[hexLocal(kid)]; ok {
			return k
		}
	}
	if len(keys) == 1 {
		for _, k := range keys {
			return k
		}
	}
	return nil
}

func hexLocal(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexdigits[c>>4], hexdigits[c&0x0F])
	}
	return string(out)
}
