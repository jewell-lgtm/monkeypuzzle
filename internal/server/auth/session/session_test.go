package session

import "testing"

func TestSecureCookieCodec_RoundTrip(t *testing.T) {
	c := NewSecureCookieCodec([]byte("a-test-secret-of-sufficient-length!!"))
	enc, err := c.Encode(4242)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4242 {
		t.Fatalf("round-trip mismatch: got %d", got)
	}
}

func TestSecureCookieCodec_RejectsForeignValue(t *testing.T) {
	a := NewSecureCookieCodec([]byte("secret-a-secret-a-secret-a-secret-a"))
	b := NewSecureCookieCodec([]byte("secret-b-secret-b-secret-b-secret-b"))
	enc, _ := a.Encode(1)
	if _, err := b.Decode(enc); err == nil {
		t.Fatal("decoded a value signed with a different key")
	}
}
