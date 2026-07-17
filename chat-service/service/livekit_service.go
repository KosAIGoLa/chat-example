package service

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/livekit/protocol/auth"
)

// LiveKitService issues access tokens for private calls and group meetings.
type LiveKitService struct {
	apiKey    string
	apiSecret string
	// url mode:
	//   "auto" (default) → same origin as the HTTP request (via nginx /rtc proxy)
	//   "ws://host:7880" → direct to LiveKit port (legacy)
	url string
}

func NewLiveKitService() *LiveKitService {
	key := os.Getenv("LIVEKIT_API_KEY")
	if key == "" {
		key = "devkey"
	}
	secret := os.Getenv("LIVEKIT_API_SECRET")
	if secret == "" {
		secret = "secret_livekit_dev_change_me_32chars!!"
	}
	url := strings.TrimSpace(os.Getenv("LIVEKIT_URL"))
	if url == "" {
		url = "auto"
	}
	return &LiveKitService{apiKey: key, apiSecret: secret, url: url}
}

// URL returns the configured static URL (may be "auto").
func (s *LiveKitService) URL() string { return s.url }

// ClientURL is the WebSocket base the browser opens.
//
// With LIVEKIT_URL=auto (recommended) this is the SAME host:port as the page
// (e.g. ws://192.168.1.102:3000). The SPA nginx proxies /rtc → livekit:7880,
// so the browser never needs a separate 7880 port or "localhost".
func (s *LiveKitService) ClientURL(r *http.Request) string {
	if s.url != "" && !strings.EqualFold(s.url, "auto") {
		return s.url
	}
	if r == nil {
		return "ws://localhost:3000"
	}
	if u := s.deriveSameOrigin(r); u != "" {
		return u
	}
	return "ws://localhost:3000"
}

// deriveSameOrigin builds ws(s)://host[:port] from the incoming API request.
// Keeps the frontend port (3000) so signaling goes through nginx, not :7880.
func (s *LiveKitService) deriveSameOrigin(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}

	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	scheme := "ws"
	if proto == "https" {
		scheme = "wss"
	}
	// host already includes port when present (e.g. "192.168.1.102:3000")
	return fmt.Sprintf("%s://%s", scheme, host)
}

// Enabled reports whether LiveKit is configured (always true with defaults).
func (s *LiveKitService) Enabled() bool {
	return s.apiKey != "" && s.apiSecret != ""
}

// PrivateRoomName builds a stable room for a 1:1 call between two users.
func PrivateRoomName(a, b string) string {
	ids := []string{a, b}
	sort.Strings(ids)
	return fmt.Sprintf("dm_%s_%s", ids[0], ids[1])
}

// GroupRoomName builds a room for a group meeting.
func GroupRoomName(groupID string) string {
	return "grp_" + strings.TrimSpace(groupID)
}

// MintToken creates a LiveKit JWT for identity to join room.
func (s *LiveKitService) MintToken(identity, displayName, room string, canPublish, canSubscribe bool) (string, error) {
	if !s.Enabled() {
		return "", errors.New("livekit not configured")
	}
	if identity == "" || room == "" {
		return "", errors.New("identity and room are required")
	}
	if displayName == "" {
		displayName = identity
	}
	at := auth.NewAccessToken(s.apiKey, s.apiSecret)
	grant := &auth.VideoGrant{
		RoomJoin:     true,
		Room:         room,
		CanPublish:   &canPublish,
		CanSubscribe: &canSubscribe,
	}
	canPublishData := true
	grant.CanPublishData = &canPublishData

	at.SetVideoGrant(grant).
		SetIdentity(identity).
		SetName(displayName).
		SetValidFor(2 * time.Hour)

	return at.ToJWT()
}
