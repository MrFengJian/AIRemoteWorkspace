package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/ai-remote/workspace/internal/domain"
)

func newTestConversationStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestConversationRepoLifecycle(t *testing.T) {
	s := newTestConversationStore(t)
	repo := NewConversationRepo(s)

	// Create two conversations (one per "host").
	if err := repo.Create(domain.Conversation{ID: "c1", HostID: "h1", HostName: "web-1", Title: "check disk usage"}); err != nil {
		t.Fatalf("create c1: %v", err)
	}
	if err := repo.Create(domain.Conversation{ID: "c2", HostID: "", HostName: "local", Title: "local question"}); err != nil {
		t.Fatalf("create c2: %v", err)
	}

	// Append a turn to c1; Touch ordering puts it first.
	if err := repo.AppendMessage("c1", "user", "看看磁盘"); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := repo.AppendMessage("c1", "assistant", "磁盘剩余 40%"); err != nil {
		t.Fatalf("append assistant: %v", err)
	}
	if err := repo.Touch("c1"); err != nil {
		t.Fatalf("touch: %v", err)
	}

	list, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].ID != "c1" {
		t.Fatalf("list = %+v, want c1 first", list)
	}

	msgs, err := repo.ListMessages("c1")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("messages = %+v, want user then assistant", msgs)
	}
	if msgs[0].Content != "看看磁盘" {
		t.Fatalf("content = %q (UTF-8 round-trip broken?)", msgs[0].Content)
	}

	// Touch on a missing conversation reports the sentinel.
	if err := repo.Touch("missing"); err == nil {
		t.Fatal("touch missing should error")
	}

	// Delete removes the conversation and its messages.
	if err := repo.Delete("c1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, err = repo.List()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 1 || list[0].ID != "c2" {
		t.Fatalf("list after delete = %+v, want only c2", list)
	}
	msgs, err = repo.ListMessages("c1")
	if err != nil {
		t.Fatalf("list messages after delete: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages after delete = %d, want 0", len(msgs))
	}
}
