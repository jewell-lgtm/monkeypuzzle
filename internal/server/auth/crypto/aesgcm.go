package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// KeySize is the required AES-256 key length in bytes.
const KeySize = 32

// AESGCMCipher is a TokenCipher using AES-256-GCM. The stored blob is
// nonce || ciphertext || tag; GCM authenticates it, so tampering fails Decrypt.
type AESGCMCipher struct {
	gcm cipher.AEAD
}

// NewAESGCMCipher returns a cipher for a 32-byte key. It fails loudly on a
// wrong key length — a short key is a silent footgun.
func NewAESGCMCipher(key []byte) (*AESGCMCipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return &AESGCMCipher{gcm: gcm}, nil
}

func (c *AESGCMCipher) Encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	return c.gcm.Seal(nonce, nonce, plain, nil), nil
}

func (c *AESGCMCipher) Decrypt(blob []byte) ([]byte, error) {
	ns := c.gcm.NonceSize()
	if len(blob) < ns {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, ciphertext := blob[:ns], blob[ns:]
	plain, err := c.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plain, nil
}

var _ TokenCipher = (*AESGCMCipher)(nil)
