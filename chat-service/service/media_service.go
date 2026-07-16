package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// MaxVoiceBytes is the maximum accepted voice file size (5 MiB).
	MaxVoiceBytes = 5 << 20
	// MaxVoiceDurationSec is the maximum allowed client-reported duration.
	MaxVoiceDurationSec = 120
)

// allowedVoiceMIME maps MIME type → preferred file extension.
// Note: Chrome/Firefox MediaRecorder often labels audio-only WebM as "video/webm".
var allowedVoiceMIME = map[string]string{
	"audio/webm":               ".webm",
	"video/webm":               ".webm", // audio-only recordings frequently use this
	"audio/ogg":                ".ogg",
	"audio/mpeg":               ".mp3",
	"audio/mp3":                ".mp3",
	"audio/mp4":                ".m4a",
	"audio/mp4a-latm":          ".m4a",
	"audio/x-m4a":              ".m4a",
	"audio/aac":                ".m4a",
	"audio/wav":                ".wav",
	"audio/x-wav":              ".wav",
	"audio/wave":               ".wav",
	"audio/x-pn-wav":           ".wav",
	"application/ogg":          ".ogg",
	"application/octet-stream": "", // resolved via filename extension below
}

// MediaService stores and serves uploaded media (voice messages).
type MediaService struct {
	dir string
}

// NewMediaService creates a media store under dir (created if missing).
func NewMediaService(dir string) (*MediaService, error) {
	if dir == "" {
		dir = "./data/voice"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create media dir: %w", err)
	}
	return &MediaService{dir: dir}, nil
}

// Dir returns the storage directory.
func (m *MediaService) Dir() string {
	return m.dir
}

// SaveVoice writes a voice file and returns a public URL path + metadata.
func (m *MediaService) SaveVoice(r io.Reader, size int64, contentType string, duration float64) (id, url, mimeType string, written int64, err error) {
	if size > MaxVoiceBytes {
		return "", "", "", 0, fmt.Errorf("file too large (max %d bytes)", MaxVoiceBytes)
	}
	if duration < 0 {
		duration = 0
	}
	if duration > MaxVoiceDurationSec {
		return "", "", "", 0, fmt.Errorf("duration too long (max %d seconds)", MaxVoiceDurationSec)
	}

	mimeType = normalizeMIME(contentType)
	ext, ok := allowedVoiceMIME[mimeType]
	if !ok || ext == "" {
		// Fall back to a safe extension when browser sends empty/unknown type.
		// Caller may also pass a filename via Content-Disposition; we only have MIME here.
		// Prefer webm (Chrome default) when type is unknown/octet-stream.
		switch {
		case strings.Contains(mimeType, "webm"):
			ext = ".webm"
			mimeType = "audio/webm"
			ok = true
		case strings.Contains(mimeType, "ogg"):
			ext = ".ogg"
			mimeType = "audio/ogg"
			ok = true
		case strings.Contains(mimeType, "mp4") || strings.Contains(mimeType, "m4a") || strings.Contains(mimeType, "aac"):
			ext = ".m4a"
			mimeType = "audio/mp4"
			ok = true
		case strings.Contains(mimeType, "mpeg") || strings.Contains(mimeType, "mp3"):
			ext = ".mp3"
			mimeType = "audio/mpeg"
			ok = true
		case strings.Contains(mimeType, "wav"):
			ext = ".wav"
			mimeType = "audio/wav"
			ok = true
		case mimeType == "" || mimeType == "application/octet-stream":
			ext = ".webm"
			mimeType = "audio/webm"
			ok = true
		}
	}
	if !ok || ext == "" {
		return "", "", "", 0, fmt.Errorf("unsupported audio type %q", contentType)
	}

	id, err = newMediaID()
	if err != nil {
		return "", "", "", 0, err
	}
	filename := id + ext
	path := filepath.Join(m.dir, filename)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// Cap reads even if Content-Length was wrong/missing.
	limited := io.LimitReader(r, MaxVoiceBytes+1)
	written, err = io.Copy(f, limited)
	if err != nil {
		_ = os.Remove(path)
		return "", "", "", 0, fmt.Errorf("write file: %w", err)
	}
	if written == 0 {
		_ = os.Remove(path)
		return "", "", "", 0, fmt.Errorf("empty file")
	}
	if written > MaxVoiceBytes {
		_ = os.Remove(path)
		return "", "", "", 0, fmt.Errorf("file too large (max %d bytes)", MaxVoiceBytes)
	}

	url = "/api/voice/" + filename
	return id, url, mimeType, written, nil
}

// ResolvePath returns the absolute filesystem path for a stored filename, or error if invalid/missing.
func (m *MediaService) ResolvePath(filename string) (string, error) {
	// Prevent path traversal: only bare filenames with known extensions.
	base := filepath.Base(filename)
	if base != filename || base == "." || base == ".." || strings.Contains(base, "..") {
		return "", fmt.Errorf("invalid filename")
	}
	ext := strings.ToLower(filepath.Ext(base))
	allowedExt := map[string]bool{
		".webm": true, ".ogg": true, ".mp3": true, ".m4a": true, ".wav": true,
	}
	if !allowedExt[ext] {
		return "", fmt.Errorf("invalid file type")
	}
	path := filepath.Join(m.dir, base)
	// Ensure resolved path stays under media dir.
	absDir, err := filepath.Abs(m.dir)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absPath, absDir+string(os.PathSeparator)) && absPath != absDir {
		return "", fmt.Errorf("invalid path")
	}
	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("not found")
	}
	return absPath, nil
}

// ContentTypeForFilename guesses Content-Type from extension.
func ContentTypeForFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".webm":
		return "audio/webm"
	case ".ogg":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	default:
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
		return "application/octet-stream"
	}
}

func normalizeMIME(ct string) string {
	ct = strings.TrimSpace(strings.ToLower(ct))
	if ct == "" {
		return ""
	}
	// Strip parameters: "audio/webm;codecs=opus" → "audio/webm"
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}

func newMediaID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// time prefix makes listings roughly chronological
	return fmt.Sprintf("%d_%s", time.Now().Unix(), hex.EncodeToString(b[:])), nil
}
