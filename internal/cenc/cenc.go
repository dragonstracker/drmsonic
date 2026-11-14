package cenc

import (
	"crypto/aes"
	"crypto/cipher"

	"drmsonic/internal/mp4"
)

// DecryptSample decrypts sample in place and returns it; the caller must not reuse the encrypted bytes.
func DecryptSample(key, iv, sample []byte, subs []mp4.Subsample) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	counter := make([]byte, 16)
	copy(counter, iv)
	stream := cipher.NewCTR(block, counter)

	if len(subs) == 0 {
		stream.XORKeyStream(sample, sample)
		return sample, nil
	}

	pos := 0
	for _, sub := range subs {
		clear := int(sub.Clear)
		if pos+clear > len(sample) {
			return sample[:pos], nil
		}
		pos += clear
		n := int(sub.Protected)
		if pos+n > len(sample) {
			n = len(sample) - pos
		}
		if n > 0 {
			stream.XORKeyStream(sample[pos:pos+n], sample[pos:pos+n])
			pos += n
		}
	}
	return sample, nil
}
