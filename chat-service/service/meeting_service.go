package service

import (
	"errors"
	"sync"
	"time"
)

// ActiveMeeting is an in-memory group conference (not a private 1:1 call).
// Members open/join freely; one active meeting per group at a time.
type ActiveMeeting struct {
	GroupID          string
	Room             string
	Media            string // audio | video
	StartedBy        string
	StartedByName    string
	StartedAt        int64
	Participants     map[string]string // userID → display name
	ParticipantCount int
}

// MeetingService tracks open group meetings (not LiveKit media itself).
type MeetingService struct {
	mu      sync.RWMutex
	byGroup map[string]*ActiveMeeting
}

func NewMeetingService() *MeetingService {
	return &MeetingService{byGroup: make(map[string]*ActiveMeeting)}
}

// Get returns a snapshot of the active meeting for a group, or nil.
func (s *MeetingService) Get(groupID string) *ActiveMeeting {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.byGroup[groupID]
	if m == nil {
		return nil
	}
	return cloneMeeting(m)
}

// Start opens a new meeting, or returns the existing one if already active.
// Returns (meeting, created, error).
func (s *MeetingService) Start(groupID, room, media, userID, username string) (*ActiveMeeting, bool, error) {
	if groupID == "" || room == "" || userID == "" {
		return nil, false, errors.New("group_id, room and user are required")
	}
	if media != "video" {
		media = "audio"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing := s.byGroup[groupID]; existing != nil {
		existing.Participants[userID] = username
		existing.ParticipantCount = len(existing.Participants)
		return cloneMeeting(existing), false, nil
	}

	m := &ActiveMeeting{
		GroupID:       groupID,
		Room:          room,
		Media:         media,
		StartedBy:     userID,
		StartedByName: username,
		StartedAt:     time.Now().Unix(),
		Participants:  map[string]string{userID: username},
	}
	m.ParticipantCount = 1
	s.byGroup[groupID] = m
	return cloneMeeting(m), true, nil
}

// Join adds a member to an existing meeting. Fails if none is active.
func (s *MeetingService) Join(groupID, userID, username string) (*ActiveMeeting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.byGroup[groupID]
	if m == nil {
		return nil, errors.New("no active meeting in this group")
	}
	if username == "" {
		username = userID
	}
	m.Participants[userID] = username
	m.ParticipantCount = len(m.Participants)
	return cloneMeeting(m), nil
}

// Leave removes a participant. If the room becomes empty, the meeting ends.
// Returns (meeting_snapshot, ended). When ended, snapshot is the closed meeting (0 participants).
func (s *MeetingService) Leave(groupID, userID string) (*ActiveMeeting, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.byGroup[groupID]
	if m == nil {
		return nil, false
	}
	delete(m.Participants, userID)
	m.ParticipantCount = len(m.Participants)
	if len(m.Participants) == 0 {
		snap := cloneMeeting(m)
		delete(s.byGroup, groupID)
		return snap, true
	}
	return cloneMeeting(m), false
}

// End force-closes a group meeting (any member may end).
func (s *MeetingService) End(groupID string) *ActiveMeeting {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.byGroup[groupID]
	if m == nil {
		return nil
	}
	delete(s.byGroup, groupID)
	return cloneMeeting(m)
}

func cloneMeeting(m *ActiveMeeting) *ActiveMeeting {
	if m == nil {
		return nil
	}
	cp := *m
	cp.Participants = make(map[string]string, len(m.Participants))
	for k, v := range m.Participants {
		cp.Participants[k] = v
	}
	cp.ParticipantCount = len(cp.Participants)
	return &cp
}
