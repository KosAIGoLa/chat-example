package model

// FrameCrypto seals/opens entire WebSocket application frames (JSON payloads).
// Implemented by service.MsgCrypto to avoid import cycles.
type FrameCrypto interface {
	// SealFrame encrypts plain application JSON into a wire envelope.
	SealFrame(plain []byte) ([]byte, error)
	// OpenFrame decrypts a wire envelope, or returns plain if unencrypted (legacy).
	OpenFrame(frame []byte) ([]byte, error)
}
