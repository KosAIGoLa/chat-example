package service

import (
	"strings"
	"testing"
)

func TestMsgCryptoRoundTrip(t *testing.T) {
	c := NewMsgCrypto("test-secret-for-unit")
	plain := "你好，这是一条私密留言 🔐"
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(enc) {
		t.Fatalf("expected encrypted prefix, got %q", enc)
	}
	if enc == plain {
		t.Fatal("ciphertext equals plaintext")
	}
	out, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if out != plain {
		t.Fatalf("got %q want %q", out, plain)
	}
}

func TestMsgCryptoIdempotentEncrypt(t *testing.T) {
	c := NewMsgCrypto("test-secret-for-unit")
	enc1, err := c.Encrypt("hello")
	if err != nil {
		t.Fatal(err)
	}
	enc2, err := c.EnsureEncrypted(enc1)
	if err != nil {
		t.Fatal(err)
	}
	if enc1 != enc2 {
		t.Fatalf("re-encrypt changed ciphertext")
	}
}

func TestMsgCryptoLegacyPlaintext(t *testing.T) {
	c := NewMsgCrypto("test-secret-for-unit")
	out, err := c.Decrypt("legacy plain message")
	if err != nil {
		t.Fatal(err)
	}
	if out != "legacy plain message" {
		t.Fatalf("got %q", out)
	}
}

func TestMsgCryptoDifferentNonces(t *testing.T) {
	c := NewMsgCrypto("test-secret-for-unit")
	a, _ := c.Encrypt("same")
	b, _ := c.Encrypt("same")
	if a == b {
		t.Fatal("expected different nonces → different ciphertext")
	}
	if !strings.HasPrefix(a, EncPrefix) || !strings.HasPrefix(b, EncPrefix) {
		t.Fatal("missing prefix")
	}
}

func TestWSFrameSealOpen(t *testing.T) {
	c := NewMsgCrypto("test-secret-for-unit")
	plain := []byte(`{"type":"private","from":"1","to":"2","content":"hi"}`)
	frame, err := c.SealFrame(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !IsWSFrame(frame) {
		t.Fatalf("expected ws_enc envelope, got %s", frame)
	}
	if string(frame) == string(plain) {
		t.Fatal("frame equals plain")
	}
	out, err := c.OpenFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(plain) {
		t.Fatalf("got %s want %s", out, plain)
	}
	// Legacy plain accepted.
	legacy, err := c.OpenFrame(plain)
	if err != nil {
		t.Fatal(err)
	}
	if string(legacy) != string(plain) {
		t.Fatal("legacy open failed")
	}
}
