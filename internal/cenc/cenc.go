package cenc

import (
	"crypto/aes"
	"crypto/cipher"

	"drmsonic/internal/mp4"
)

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
