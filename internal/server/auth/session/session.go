// Package session manages mp server's signed, encrypted browser session: a
// cookie carrying the authenticated user's id. It is the human web login state,
// distinct from the OAuth bearer tokens agents use on the MCP surface.
package session

import (
	"crypto/sha256"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

// CookieName is the session cookie name.
const CookieName = "mp_session"

// Codec encodes/decodes a user id to/from an opaque cookie value.
type Codec interface {
	Encode(userID int64) (string, error)
	Decode(value string) (int64, error)
}

// SecureCookieCodec signs and encrypts the session value with gorilla/securecookie.
type SecureCookieCodec struct {
	sc *securecookie.SecureCookie
}

// NewSecureCookieCodec derives independent HMAC and AES keys from secret (which
// should be 32+ random bytes) and returns a Codec.
func NewSecureCookieCodec(secret []byte) *SecureCookieCodec {
	hashKey := sha256.Sum256(append([]byte("mp-session-hash:"), secret...))
	blockKey := sha256.Sum256(append([]byte("mp-session-block:"), secret...))
	return &SecureCookieCodec{sc: securecookie.New(hashKey[:], blockKey[:])}
}

func (c *SecureCookieCodec) Encode(userID int64) (string, error) {
	return c.sc.Encode(CookieName, userID)
}

func (c *SecureCookieCodec) Decode(value string) (int64, error) {
	var userID int64
	if err := c.sc.Decode(CookieName, value, &userID); err != nil {
		return 0, err
	}
	return userID, nil
}

// Set writes the session cookie (HttpOnly, SameSite=Lax, 30-day expiry).
func Set(w http.ResponseWriter, value string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})
}

// Clear expires the session cookie.
func Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// Read returns the raw session cookie value, or ("", http.ErrNoCookie).
func Read(r *http.Request) (string, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return "", err
	}
	return c.Value, nil
}

var _ Codec = (*SecureCookieCodec)(nil)
