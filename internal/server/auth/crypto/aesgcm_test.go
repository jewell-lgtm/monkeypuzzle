package crypto

import (
	"bytes"
	"testing"
)

func testKey() []byte {
	k := make([]byte, KeySize)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestAESGCM_RoundTrip(t *testing.T) {
	c, err := NewAESGCMCipher(testKey())
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("gho_secrettoken_123")
	blob, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, plain) {
		t.Fatal("ciphertext leaks plaintext")
	}
	got, err := c.Decrypt(blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round-trip mismatch: %q != %q", got, plain)
	}
}

func TestAESGCM_TamperDetected(t *testing.T) {
	c, _ := NewAESGCMCipher(testKey())
	blob, _ := c.Encrypt([]byte("secret"))
	blob[len(blob)-1] ^= 0xff // flip a byte in the tag
	if _, err := c.Decrypt(blob); err == nil {
		t.Fatal("expected tamper to fail decryption")
	}
}

func TestAESGCM_RejectsWrongKeySize(t *testing.T) {
	if _, err := NewAESGCMCipher(make([]byte, 16)); err == nil {
		t.Fatal("expected error for 16-byte key")
	}
}

func TestAESGCM_NonceIsRandom(t *testing.T) {
	c, _ := NewAESGCMCipher(testKey())
	a, _ := c.Encrypt([]byte("x"))
	b, _ := c.Encrypt([]byte("x"))
	if bytes.Equal(a, b) {
		t.Fatal("same plaintext produced identical ciphertext (nonce reuse)")
	}
}
