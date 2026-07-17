package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
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

const (
	// MaxAvatarBytes 2 MiB.
	MaxAvatarBytes = 2 << 20
	// MinAvatarPixels rejects accidental solid 1×1 / 8×8 junk uploads.
	MinAvatarPixels = 32
)

var allowedAvatarMIME = map[string]string{
	"image/jpeg": ".jpg",
	"image/jpg":  ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// avatarDir returns MEDIA_DIR/avatars.
func (m *MediaService) avatarDir() (string, error) {
	dir := filepath.Join(m.dir, "avatars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// SaveAvatar stores an image as avatars/{userID}{ext}, replacing previous files for that user.
// Returns public path "/api/avatar/{userID}".
func (m *MediaService) SaveAvatar(userID uint, r io.Reader, contentType string) (publicPath string, mimeType string, written int64, err error) {
	mimeType = normalizeMIME(contentType)
	ext, ok := allowedAvatarMIME[mimeType]
	if !ok {
		// Guess from common prefixes.
		switch {
		case strings.Contains(mimeType, "jpeg") || strings.Contains(mimeType, "jpg"):
			ext, mimeType, ok = ".jpg", "image/jpeg", true
		case strings.Contains(mimeType, "png"):
			ext, mimeType, ok = ".png", "image/png", true
		case strings.Contains(mimeType, "webp"):
			ext, mimeType, ok = ".webp", "image/webp", true
		case strings.Contains(mimeType, "gif"):
			ext, mimeType, ok = ".gif", "image/gif", true
		}
	}
	if !ok || ext == "" {
		return "", "", 0, fmt.Errorf("unsupported image type %q (use jpeg/png/webp/gif)", contentType)
	}

	dir, err := m.avatarDir()
	if err != nil {
		return "", "", 0, err
	}

	// Remove any previous avatar for this user (any extension).
	uid := fmt.Sprintf("%d", userID)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, uid+".") || name == uid {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}

	filename := uid + ext
	path := filepath.Join(dir, filename)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", "", 0, fmt.Errorf("create avatar: %w", err)
	}
	defer f.Close()

	limited := io.LimitReader(r, MaxAvatarBytes+1)
	written, err = io.Copy(f, limited)
	if err != nil {
		_ = os.Remove(path)
		return "", "", 0, fmt.Errorf("write avatar: %w", err)
	}
	if written == 0 {
		_ = os.Remove(path)
		return "", "", 0, fmt.Errorf("empty file")
	}
	if written > MaxAvatarBytes {
		_ = os.Remove(path)
		return "", "", 0, fmt.Errorf("file too large (max %d bytes)", MaxAvatarBytes)
	}

	// Reject tiny junk (e.g. 8×8 solid red PNG) that would look like a blank circle.
	// webp may not decode with stdlib — skip dimension check if DecodeConfig fails.
	if rf, err := os.Open(path); err == nil {
		cfg, _, decErr := image.DecodeConfig(rf)
		_ = rf.Close()
		if decErr == nil {
			if cfg.Width < MinAvatarPixels || cfg.Height < MinAvatarPixels {
				_ = os.Remove(path)
				return "", "", 0, fmt.Errorf("image too small (min %dx%d)", MinAvatarPixels, MinAvatarPixels)
			}
		}
	}

	publicPath = "/api/avatar/" + uid
	return publicPath, mimeType, written, nil
}

// ResolveAvatarPath finds avatars/{userID}.* on disk.
func (m *MediaService) ResolveAvatarPath(userID string) (absPath string, contentType string, err error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || strings.Contains(userID, "..") || strings.ContainsAny(userID, "/\\") {
		return "", "", fmt.Errorf("invalid user id")
	}
	dir, err := m.avatarDir()
	if err != nil {
		return "", "", err
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif"} {
		p := filepath.Join(dir, userID+ext)
		if st, e := os.Stat(p); e == nil && !st.IsDir() {
			ct := "image/jpeg"
			switch ext {
			case ".png":
				ct = "image/png"
			case ".webp":
				ct = "image/webp"
			case ".gif":
				ct = "image/gif"
			}
			return p, ct, nil
		}
	}
	return "", "", fmt.Errorf("not found")
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
