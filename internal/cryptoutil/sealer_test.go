package cryptoutil

import (
	"bytes"
	"testing"
)

func TestSealerRoundTripAndContextBinding(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	sealer, err := NewSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := sealer.Seal([]byte("secret"), "setting:test")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := sealer.Open(envelope, "setting:test")
	if err != nil || string(plain) != "secret" {
		t.Fatalf("round trip failed: %q %v", plain, err)
	}
	if _, err := sealer.Open(envelope, "setting:other"); err == nil {
		t.Fatal("expected associated-data mismatch")
	}
}
