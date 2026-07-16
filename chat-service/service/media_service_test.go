package service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveVoiceAcceptsBrowserMIMETypes(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMediaService(dir)
	if err != nil {
		t.Fatal(err)
	}

	cases := []string{
		"video/webm",
		"video/webm;codecs=opus",
		"audio/webm;codecs=opus",
		"audio/webm",
		"audio/mp4",
		"application/octet-stream",
		"",
	}
	payload := []byte("dummy-audio-payload-for-voice-test")
	for _, ct := range cases {
		_, url, mime, n, err := m.SaveVoice(bytes.NewReader(payload), int64(len(payload)), ct, 1.2)
		if err != nil {
			t.Fatalf("SaveVoice(%q) unexpected error: %v", ct, err)
		}
		if url == "" || n == 0 || mime == "" {
			t.Fatalf("SaveVoice(%q) incomplete result url=%q mime=%q n=%d", ct, url, mime, n)
		}
		// File must exist on disk
		name := filepath.Base(url)
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("file missing for %q: %v", ct, err)
		}
	}
}

func TestSaveVoiceRejectsUnknownType(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMediaService(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err = m.SaveVoice(bytes.NewReader([]byte("xxx")), 3, "image/png", 1)
	if err == nil {
		t.Fatal("expected error for image/png")
	}
}
