package arcad

import (
	"testing"
	"time"
)

func TestSessionRoundTrip(t *testing.T) {
	m := NewSessionManager("secret")
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	token, err := m.Encode(Session{UserID: "u1", ExpiresAt: expires})
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	session, err := m.Decode(token)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if session.UserID != "u1" {
		t.Fatalf("unexpected user id: %q", session.UserID)
	}
	if !session.ExpiresAt.Equal(expires) {
		t.Fatalf("unexpected expiry: %s != %s", session.ExpiresAt, expires)
	}
}

func TestSessionDecodeRejectsTamperedToken(t *testing.T) {
	m := NewSessionManager("secret")
	token, err := m.Encode(Session{UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	if _, err := m.Decode(token + "x"); err == nil {
		t.Fatalf("expected decode error for tampered token")
	}
}
