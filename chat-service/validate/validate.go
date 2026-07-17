// Package validate provides shared input sanitization and legitimacy checks
// for HTTP/API parameters (path, query, JSON body).
package validate

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Common length limits.
const (
	MaxUsernameLen  = 50
	MinUsernameLen  = 3
	MaxPasswordLen  = 72 // bcrypt limit
	MinPasswordLen  = 6
	MaxGroupIDLen   = 64
	MinGroupIDLen   = 3
	MaxGroupNameLen = 40
	MinGroupNameLen = 2
	MaxSearchQLen   = 64
	MaxRoomLen      = 128
	MaxGreetingLen  = 80
	MaxLimit        = 100
	DefaultLimit    = 20
)

var (
	// username: letters, digits, underscore; starts with letter/digit.
	usernameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_]{2,49}$`)
	// group id: letters, digits, _ -
	groupIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,63}$`)
	// numeric user id string
	digitIDRe = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
	// room names: alnum + _ -
	roomRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)
	// red packet / message id hex-ish
	idHexRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,64}$`)
)

// ErrInvalid is returned for failed validation (message is client-safe).
type ErrInvalid struct {
	Msg string
}

func (e ErrInvalid) Error() string { return e.Msg }

func bad(msg string) error { return ErrInvalid{Msg: msg} }

// Clean trims space and strips NUL / most control characters (keeps \n \t for text).
func Clean(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == 0 || r == '\uFEFF' {
			continue
		}
		// Drop C0 controls except tab/newline/carriage return.
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			continue
		}
		if r == 0x7f {
			continue
		}
		// Drop other non-character / line/paragraph separators used in injection.
		if r == '\u2028' || r == '\u2029' {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// CleanSingleLine is Clean without newlines/tabs (names, ids, usernames).
func CleanSingleLine(s string) string {
	s = Clean(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}

// HasControl reports remaining disallowed control runes.
func HasControl(s string) bool {
	for _, r := range s {
		if r == 0 || (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f {
			return true
		}
	}
	return false
}

// Username validates register / profile username (strict).
func Username(raw string) (string, error) {
	s := CleanSingleLine(raw)
	if s == "" {
		return "", bad("username is required")
	}
	if !utf8.ValidString(s) {
		return "", bad("username contains invalid UTF-8")
	}
	n := utf8.RuneCountInString(s)
	if n < MinUsernameLen || n > MaxUsernameLen {
		return "", bad(fmt.Sprintf("username must be %d–%d characters", MinUsernameLen, MaxUsernameLen))
	}
	if !usernameRe.MatchString(s) {
		return "", bad("username: letters, digits, underscore; 3–50 chars; start with letter or digit")
	}
	return s, nil
}

// LoginIdentity is more permissive for login (legacy accounts may not match strict username rules).
func LoginIdentity(raw string) (string, error) {
	s := CleanSingleLine(raw)
	if s == "" {
		return "", bad("username is required")
	}
	if !utf8.ValidString(s) {
		return "", bad("username contains invalid UTF-8")
	}
	n := utf8.RuneCountInString(s)
	if n < 1 || n > MaxUsernameLen {
		return "", bad(fmt.Sprintf("username must be 1–%d characters", MaxUsernameLen))
	}
	if HasControl(s) {
		return "", bad("username contains illegal characters")
	}
	return s, nil
}

// Password validates password length only (content free-form).
func Password(raw string, required bool) (string, error) {
	// Do not strip interior spaces from passwords — only trim ends.
	s := strings.TrimSpace(raw)
	if s == "" {
		if required {
			return "", bad("password is required")
		}
		return "", nil
	}
	if strings.ContainsRune(s, 0) {
		return "", bad("password contains illegal characters")
	}
	// Use byte length for bcrypt compatibility messaging.
	if len(s) < MinPasswordLen {
		return "", bad(fmt.Sprintf("password must be at least %d characters", MinPasswordLen))
	}
	if len(s) > MaxPasswordLen {
		return "", bad(fmt.Sprintf("password must be at most %d characters", MaxPasswordLen))
	}
	return s, nil
}

// GroupID validates optional or required public group id.
func GroupID(raw string, required bool) (string, error) {
	s := CleanSingleLine(raw)
	if s == "" {
		if required {
			return "", bad("group_id is required")
		}
		return "", nil
	}
	if !utf8.ValidString(s) {
		return "", bad("group_id contains invalid UTF-8")
	}
	n := utf8.RuneCountInString(s)
	if n < MinGroupIDLen || n > MaxGroupIDLen {
		return "", bad(fmt.Sprintf("group_id must be %d–%d characters", MinGroupIDLen, MaxGroupIDLen))
	}
	if !groupIDRe.MatchString(s) {
		return "", bad("group_id: start with letter/digit; only letters, digits, _ or -")
	}
	// Block reserved path segments.
	switch strings.ToLower(s) {
	case "search", "join", "leave", "members", "new", "create", "me", "admin":
		return "", bad("group_id is reserved")
	}
	return s, nil
}

// GroupName validates display name (required for create).
func GroupName(raw string) (string, error) {
	s := CleanSingleLine(raw)
	if s == "" {
		return "", bad("群名称不能为空")
	}
	if !utf8.ValidString(s) {
		return "", bad("群名称包含非法字符")
	}
	n := utf8.RuneCountInString(s)
	if n < MinGroupNameLen {
		return "", bad(fmt.Sprintf("群名称至少 %d 个字符", MinGroupNameLen))
	}
	if n > MaxGroupNameLen {
		return "", bad(fmt.Sprintf("群名称最多 %d 个字符", MaxGroupNameLen))
	}
	// Reject pure punctuation / whitespace-only already handled.
	hasLetterOrDigit := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasLetterOrDigit = true
			break
		}
	}
	if !hasLetterOrDigit {
		return "", bad("群名称需包含文字或数字")
	}
	return s, nil
}

// SearchQuery validates fuzzy search q.
func SearchQuery(raw string) (string, error) {
	s := CleanSingleLine(raw)
	if s == "" {
		return "", nil
	}
	if utf8.RuneCountInString(s) > MaxSearchQLen {
		return "", bad(fmt.Sprintf("search query too long (max %d)", MaxSearchQLen))
	}
	if HasControl(s) {
		return "", bad("search query contains illegal characters")
	}
	return s, nil
}

// Limit clamps pagination limit.
func Limit(raw string, def, max int) (int, error) {
	if def <= 0 {
		def = DefaultLimit
	}
	if max <= 0 {
		max = MaxLimit
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, bad("limit must be an integer")
	}
	if n < 1 {
		return 0, bad("limit must be >= 1")
	}
	if n > max {
		n = max
	}
	return n, nil
}

// UserIDStr validates numeric user id path/query.
func UserIDStr(raw string, required bool) (string, error) {
	s := CleanSingleLine(raw)
	if s == "" {
		if required {
			return "", bad("user_id is required")
		}
		return "", nil
	}
	if !digitIDRe.MatchString(s) {
		return "", bad("user_id must be a positive integer")
	}
	return s, nil
}

// PeerID is alias of UserIDStr for private chat peer.
func PeerID(raw string, required bool) (string, error) {
	s, err := UserIDStr(raw, required)
	if err != nil {
		if required {
			return "", bad("peer_id is required and must be a positive integer")
		}
		return "", err
	}
	return s, nil
}

// Room validates LiveKit room name when client-supplied.
func Room(raw string, required bool) (string, error) {
	s := CleanSingleLine(raw)
	if s == "" {
		if required {
			return "", bad("room is required")
		}
		return "", nil
	}
	if utf8.RuneCountInString(s) > MaxRoomLen {
		return "", bad("room name too long")
	}
	if !roomRe.MatchString(s) {
		return "", bad("room: letters, digits, _ or - only")
	}
	return s, nil
}

// CallType private|group
func CallType(raw string) (string, error) {
	s := strings.ToLower(CleanSingleLine(raw))
	if s == "" {
		s = "private"
	}
	switch s {
	case "private", "group":
		return s, nil
	default:
		return "", bad("type must be private or group")
	}
}

// Media audio|video
func Media(raw string) (string, error) {
	s := strings.ToLower(CleanSingleLine(raw))
	if s == "" {
		return "audio", nil
	}
	switch s {
	case "audio", "video":
		return s, nil
	default:
		return "", bad("media must be audio or video")
	}
}

// MeetingAction start|join|leave|end
func MeetingAction(raw string) (string, error) {
	s := strings.ToLower(CleanSingleLine(raw))
	switch s {
	case "start", "join", "leave", "end":
		return s, nil
	default:
		return "", bad("action must be start, join, leave, or end")
	}
}

// CallAction invite|accept|reject|end|cancel
func CallAction(raw string) (string, error) {
	s := strings.ToLower(CleanSingleLine(raw))
	switch s {
	case "invite", "accept", "reject", "end", "cancel":
		return s, nil
	default:
		return "", bad("action must be invite, accept, reject, end, or cancel")
	}
}

// ResourceID validates red-packet / message ids.
func ResourceID(raw string, required bool) (string, error) {
	s := CleanSingleLine(raw)
	if s == "" {
		if required {
			return "", bad("id is required")
		}
		return "", nil
	}
	if !idHexRe.MatchString(s) {
		return "", bad("id format is invalid")
	}
	return s, nil
}

// Greeting short red-packet text.
func Greeting(raw string) (string, error) {
	s := CleanSingleLine(raw)
	if s == "" {
		return "恭喜发财", nil
	}
	if utf8.RuneCountInString(s) > MaxGreetingLen {
		return "", bad(fmt.Sprintf("greeting max %d characters", MaxGreetingLen))
	}
	return s, nil
}

// HistoryType private|group
func HistoryType(raw string) (string, error) {
	s := strings.ToLower(CleanSingleLine(raw))
	switch s {
	case "private", "group", "":
		return s, nil
	default:
		return "", bad("type must be private or group")
	}
}

// NonNegInt64 parses non-negative int64.
func NonNegInt64(raw string, field string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, bad(field + " must be a non-negative integer")
	}
	return n, nil
}

// PositiveInt64 parses positive int64.
func PositiveInt64(n int64, field string) error {
	if n <= 0 {
		return bad(field + " must be > 0")
	}
	return nil
}

// PositiveInt parses positive int.
func PositiveInt(n int, field string) error {
	if n <= 0 {
		return bad(field + " must be > 0")
	}
	return nil
}

// JSONBody ensures bind errors map to clean messages.
func JSONBody(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	// Gin binding errors can be noisy; keep short.
	if strings.Contains(msg, "EOF") || strings.Contains(msg, "unexpected end") {
		return bad("request body is required")
	}
	if strings.Contains(msg, "cannot unmarshal") {
		return bad("invalid JSON body")
	}
	return bad("invalid request body")
}

// IsInvalid reports ErrInvalid.
func IsInvalid(err error) bool {
	var e ErrInvalid
	return errors.As(err, &e)
}
