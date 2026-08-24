package application

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/ai-remote/workspace/internal/domain"
)

// HostOfSessionResolver resolves the host behind a terminal session id
// (implemented by infrastructure/ssh.Manager).
type HostOfSessionResolver interface {
	HostOfSession(sessionID string) (domain.Host, bool)
}

// ConversationService ties live agent chats to persisted conversations:
// each terminal session maps to at most one conversation (created lazily on
// the first turn); every completed turn is appended to storage; resuming a
// conversation re-points a session at it. It also implements agent.TurnSink.
type ConversationService struct {
	repo  ConversationRepository
	hosts HostOfSessionResolver

	mu     sync.Mutex
	active map[string]string // terminal sessionID → conversationID
}

// NewConversationService wires the service. hosts may be nil (local-only).
func NewConversationService(repo ConversationRepository, hosts HostOfSessionResolver) *ConversationService {
	return &ConversationService{
		repo:   repo,
		hosts:  hosts,
		active: make(map[string]string),
	}
}

// EnsureMapping returns the session's conversation id, creating (and mapping)
// a new persisted conversation when none is active. firstMessage seeds the
// title.
func (s *ConversationService) EnsureMapping(sessionID, firstMessage string) (string, error) {
	s.mu.Lock()
	if convID, ok := s.active[sessionID]; ok {
		s.mu.Unlock()
		return convID, nil
	}
	s.mu.Unlock()

	hostID, hostName := s.sessionHost(sessionID)
	conv := domain.Conversation{
		ID:       newConversationID(),
		HostID:   hostID,
		HostName: hostName,
		Title:    truncateRunes(strings.TrimSpace(firstMessage), 60),
	}
	if err := s.repo.Create(conv); err != nil {
		return "", fmt.Errorf("create conversation: %w", err)
	}

	s.mu.Lock()
	// Another chat could have raced us; first mapping wins, the extra row is
	// harmless and titleless-empty conversations are filtered in the UI.
	if existing, ok := s.active[sessionID]; ok {
		s.mu.Unlock()
		return existing, nil
	}
	s.active[sessionID] = conv.ID
	s.mu.Unlock()
	return conv.ID, nil
}

// RecordTurn appends a completed (user, assistant) turn to the session's
// conversation. Implements the agent runtime's TurnSink; errors are logged,
// never surfaced — persistence must not break chatting.
func (s *ConversationService) RecordTurn(sessionID, user, assistant string) {
	s.mu.Lock()
	convID, ok := s.active[sessionID]
	s.mu.Unlock()
	if !ok {
		return
	}
	if err := s.repo.AppendMessage(convID, "user", user); err != nil {
		log.Printf("[ConversationService] append user message: %v", err)
		return
	}
	if err := s.repo.AppendMessage(convID, "assistant", assistant); err != nil {
		log.Printf("[ConversationService] append assistant message: %v", err)
		return
	}
	if err := s.repo.Touch(convID); err != nil {
		log.Printf("[ConversationService] touch conversation: %v", err)
	}
}

// SetActive points a session at an existing conversation (resume).
func (s *ConversationService) SetActive(sessionID, conversationID string) {
	s.mu.Lock()
	s.active[sessionID] = conversationID
	s.mu.Unlock()
}

// ActiveConversation returns the session's active conversation id, if any.
func (s *ConversationService) ActiveConversation(sessionID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	convID, ok := s.active[sessionID]
	return convID, ok
}

// ClearMapping detaches a session from its conversation (new chat / tab
// closed). The conversation stays persisted and resumable.
func (s *ConversationService) ClearMapping(sessionID string) {
	s.mu.Lock()
	delete(s.active, sessionID)
	s.mu.Unlock()
}

// List returns all persisted conversations, newest first.
func (s *ConversationService) List() ([]domain.Conversation, error) {
	return s.repo.List()
}

// Messages returns a conversation's messages in order.
func (s *ConversationService) Messages(conversationID string) ([]domain.ConversationMessage, error) {
	return s.repo.ListMessages(conversationID)
}

// Delete removes a conversation. If it was the active conversation of a
// session, that session's mapping is dropped and the session id returned so
// the caller can also clear the runtime's in-memory context.
func (s *ConversationService) Delete(conversationID string) (affectedSession string, err error) {
	if err := s.repo.Delete(conversationID); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for sid, convID := range s.active {
		if convID == conversationID {
			delete(s.active, sid)
			return sid, nil
		}
	}
	return "", nil
}

// sessionHost resolves the host identity behind a terminal session:
// local-terminal sessions (id prefix "local-") map to HostID "" / name
// "local"; SSH sessions resolve through the connection manager.
func (s *ConversationService) sessionHost(sessionID string) (hostID, hostName string) {
	if strings.HasPrefix(sessionID, "local-") {
		return "", "local"
	}
	if s.hosts != nil {
		if host, ok := s.hosts.HostOfSession(sessionID); ok {
			return host.ID, host.Name
		}
	}
	return "", "unknown"
}

func newConversationID() string { return newID() }

// truncateRunes caps a string to n runes (titles keep full characters, not
// bytes — Chinese titles would otherwise be cut mid-character).
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
