package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// EncPrefix marks ciphertext stored in ChatMessageDTO.Content.
	EncPrefix = "enc:v1:"
	// NonceSize for AES-GCM.
	gcmNonceSize = 12
	// WS frame envelope type on the wire.
	wsEncType = "ws_enc"
)

// MsgCrypto encrypts/decrypts chat message body (AES-256-GCM).
// Key is derived from MSG_CRYPTO_KEY (or JWT_SECRET) via SHA-256.
type MsgCrypto struct {
	key []byte // 32 bytes
}

// NewMsgCrypto builds a crypto helper from an explicit secret, or env, or default.
func NewMsgCrypto(secret string) *MsgCrypto {
	if secret == "" {
		secret = os.Getenv("MSG_CRYPTO_KEY")
	}
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	if secret == "" {
		secret = "change-me-in-production"
	}
	sum := sha256.Sum256([]byte(secret))
	return &MsgCrypto{key: sum[:]}
}

// KeyBase64 returns the raw AES key for authenticated clients (Web Crypto import).
func (c *MsgCrypto) KeyBase64() string {
	return base64.StdEncoding.EncodeToString(c.key)
}

// Algorithm describes the scheme for the client.
func (c *MsgCrypto) Algorithm() string {
	return "AES-GCM"
}

// Version is the current wire format version.
func (c *MsgCrypto) Version() int {
	return 1
}

// Encrypt encrypts plaintext and returns EncPrefix + base64(nonce||ciphertext).
func (c *MsgCrypto) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if IsEncrypted(plaintext) {
		return plaintext, nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	// Seal appends ciphertext+tag to nonce for a single blob.
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return EncPrefix + base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt. Plaintext without prefix is returned as-is (legacy).
func (c *MsgCrypto) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !IsEncrypted(ciphertext) {
		return ciphertext, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, EncPrefix))
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// IsEncrypted reports whether content uses our wire format.
func IsEncrypted(content string) bool {
	return strings.HasPrefix(content, EncPrefix)
}

// EnsureEncrypted encrypts content if it is still plaintext.
func (c *MsgCrypto) EnsureEncrypted(content string) (string, error) {
	if content == "" || IsEncrypted(content) {
		return content, nil
	}
	return c.Encrypt(content)
}

// wsFrameEnvelope is the wire format for fully encrypted WebSocket frames.
// {"type":"ws_enc","v":1,"data":"<base64(nonce||ciphertext||tag)>"}
type wsFrameEnvelope struct {
	Type string `json:"type"`
	V    int    `json:"v"`
	Data string `json:"data"`
}

// SealFrame encrypts an entire application JSON payload for WebSocket transport.
func (c *MsgCrypto) SealFrame(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return plain, nil
	}
	// Already an envelope — do not double-wrap.
	if IsWSFrame(plain) {
		return plain, nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	env := wsFrameEnvelope{
		Type: wsEncType,
		V:    1,
		Data: base64.StdEncoding.EncodeToString(sealed),
	}
	return json.Marshal(env)
}

// OpenFrame decrypts a WebSocket wire envelope. Legacy plain JSON is returned as-is.
func (c *MsgCrypto) OpenFrame(frame []byte) ([]byte, error) {
	if len(frame) == 0 {
		return frame, nil
	}
	if !IsWSFrame(frame) {
		// Backward compatible: accept plaintext frames during migration.
		return frame, nil
	}
	var env wsFrameEnvelope
	if err := json.Unmarshal(frame, &env); err != nil {
		return nil, fmt.Errorf("ws frame envelope: %w", err)
	}
	raw, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil {
		return nil, fmt.Errorf("ws frame data: %w", err)
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("ws frame too short")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("ws frame decrypt: %w", err)
	}
	return plain, nil
}

// IsWSFrame reports whether raw bytes look like an encrypted WS envelope.
func IsWSFrame(frame []byte) bool {
	// Fast path without full unmarshal.
	s := strings.TrimSpace(string(frame))
	return strings.HasPrefix(s, `{"type":"ws_enc"`) ||
		strings.HasPrefix(s, `{"type": "ws_enc"`) ||
		(strings.Contains(s, `"type":"ws_enc"`) && strings.Contains(s, `"data"`))
}
