package cenc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"testing"

	"drmsonic/internal/mp4"
)

func encrypt(key, iv, plain []byte, subs []mp4.Subsample) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	counter := make([]byte, 16)
	copy(counter, iv)
	stream := cipher.NewCTR(block, counter)
	out := make([]byte, len(plain))
	copy(out, plain)
	if len(subs) == 0 {
		stream.XORKeyStream(out, out)
		return out
	}
	pos := 0
	for _, sub := range subs {
		pos += int(sub.Clear)
		n := int(sub.Protected)
		if pos+n > len(out) {
			n = len(out) - pos
		}
		if n > 0 {
			stream.XORKeyStream(out[pos:pos+n], out[pos:pos+n])
			pos += n
		}
	}
	return out
}

func TestDecryptSubsamples(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 16)
	iv := bytes.Repeat([]byte{0x01}, 8)
	plain := make([]byte, 400)
	for i := range plain {
		plain[i] = byte(i)
	}
	subs := []mp4.Subsample{
		{Clear: 5, Protected: 100},
		{Clear: 7, Protected: 33},
		{Clear: 3, Protected: 200},
	}

	enc := encrypt(key, iv, plain, subs)
	if bytes.Equal(enc, plain) {
		t.Fatal("encryption did not change the sample")
	}

	got, err := DecryptSample(key, iv, enc, subs)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("subsample decryption does not round trip")
	}
}

func TestDecryptWholeSample(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 16)
	iv := bytes.Repeat([]byte{0x02}, 16)
	plain := bytes.Repeat([]byte("nalu"), 64)

	enc := encrypt(key, iv, plain, nil)
	got, err := DecryptSample(key, iv, enc, nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("full sample decryption does not round trip")
	}
}

func TestDecryptKeepsClearBytes(t *testing.T) {
	key := bytes.Repeat([]byte{0x07}, 16)
	iv := bytes.Repeat([]byte{0x03}, 8)
	subs := []mp4.Subsample{{Clear: 16, Protected: 16}}

	enc := encrypt(key, iv, bytes.Repeat([]byte{0xAA}, 32), subs)
	got, err := DecryptSample(key, iv, enc, subs)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	for i := 0; i < 16; i++ {
		if got[i] != 0xAA {
			t.Fatalf("clear byte %d was modified", i)
		}
	}
}
