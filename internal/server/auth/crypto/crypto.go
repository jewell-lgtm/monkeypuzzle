// Package crypto encrypts the GitHub OAuth tokens mp server stores at rest.
// Tokens are encrypted at login (web process) and only decrypted in the worker
// when it calls the GitHub API on the user's behalf.
package crypto

// TokenCipher encrypts and decrypts opaque secret bytes (GitHub tokens).
type TokenCipher interface {
	Encrypt(plain []byte) ([]byte, error)
	Decrypt(blob []byte) ([]byte, error)
}
