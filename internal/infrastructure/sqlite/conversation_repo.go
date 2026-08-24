package sqlite

import (
	"errors"
	"sort"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// ErrConversationNotFound is returned when a conversation id is unknown.
var ErrConversationNotFound = errors.New("conversation not found")

// ConversationRepo implements application.ConversationRepository over GORM.
// Follows the host_repo patterns: sentinel errors, UTC timestamps, and
// application-level cleanup of child rows (no DB foreign keys).
type ConversationRepo struct {
	store *Store
}

// NewConversationRepo builds a ConversationRepo.
func NewConversationRepo(store *Store) *ConversationRepo {
	return &ConversationRepo{store: store}
}

// List returns all conversations, newest first. Sorted in Go: the pure-Go
// sqlite driver stores timestamps with varying timezone representations, so
// SQL-level ORDER BY on the column is not reliable.
func (r *ConversationRepo) List() ([]domain.Conversation, error) {
	var models []conversationModel
	if err := r.store.db.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Conversation, 0, len(models))
	for _, m := range models {
		out = append(out, conversationFromModel(m))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Create stores a new conversation.
func (r *ConversationRepo) Create(c domain.Conversation) error {
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	return r.store.db.Save(&conversationModel{
		ID:        c.ID,
		HostID:    c.HostID,
		HostName:  c.HostName,
		Title:     c.Title,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}).Error
}

// Touch refreshes a conversation's UpdatedAt (after a new turn).
func (r *ConversationRepo) Touch(conversationID string) error {
	res := r.store.db.Model(&conversationModel{}).
		Where("id = ?", conversationID).
		Update("updated_at", time.Now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrConversationNotFound
	}
	return nil
}

// ListMessages returns a conversation's user/assistant messages in order.
func (r *ConversationRepo) ListMessages(conversationID string) ([]domain.ConversationMessage, error) {
	var models []conversationMessageModel
	if err := r.store.db.
		Where("conversation_id = ?", conversationID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.ConversationMessage, 0, len(models))
	for _, m := range models {
		out = append(out, domain.ConversationMessage{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
		})
	}
	return out, nil
}

// AppendMessage stores one message in a conversation.
func (r *ConversationRepo) AppendMessage(conversationID string, role, content string) error {
	return r.store.db.Create(&conversationMessageModel{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		CreatedAt:      time.Now().UTC(),
	}).Error
}

// Delete removes a conversation and its messages (children first).
func (r *ConversationRepo) Delete(conversationID string) error {
	if err := r.store.db.
		Where("conversation_id = ?", conversationID).
		Delete(&conversationMessageModel{}).Error; err != nil {
		return err
	}
	return r.store.db.Delete(&conversationModel{}, "id = ?", conversationID).Error
}

func conversationFromModel(m conversationModel) domain.Conversation {
	return domain.Conversation{
		ID:        m.ID,
		HostID:    m.HostID,
		HostName:  m.HostName,
		Title:     m.Title,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
