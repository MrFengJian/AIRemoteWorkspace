package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/ai-remote/workspace/internal/domain"
)

func TestRunCommand(t *testing.T) {
	// Fake OpenAI-compatible endpoint: returns a fixed summary.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"压缩摘要：排查了 nginx 502"}}]}`))
	}))
	defer srv.Close()

	resolver := func(string, string) (domain.LLMEndpoint, error) {
		return domain.LLMEndpoint{BaseURL: srv.URL, Model: "test"}, nil
	}
	sink := &memTurnSink{turns: [][2]string{}}
	r := &Runtime{llm: llmResolverFunc(resolver), sink: sink, histories: map[string][]*schema.Message{}}
	r.histories["s1"] = []*schema.Message{
		schema.UserMessage("nginx 502 了"),
		schema.AssistantMessage("排查发现上游超时，已恢复", nil),
	}

	if _, err := r.RunCommand(context.Background(), "s1", "p", "m", "token"); err != nil {
		t.Fatalf("token: %v", err)
	}
	// token appends the command exchange to the existing two messages.
	if len(r.histories["s1"]) != 4 {
		t.Fatalf("token: expected 4 messages, got %d", len(r.histories["s1"]))
	}

	result, err := r.RunCommand(context.Background(), "s1", "p", "m", "compact")
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if !strings.Contains(result, "压缩摘要：排查了 nginx 502") {
		t.Fatalf("compact result wrong: %q", result)
	}
	// compact REPLACES memory with the command exchange.
	h := r.histories["s1"]
	if len(h) != 2 || h[0].Content != "/compact" || !strings.Contains(h[1].Content, "压缩摘要") {
		t.Fatalf("compact memory wrong: %+v", h)
	}

	// summary appends without replacing.
	before := len(r.histories["s1"])
	if _, err := r.RunCommand(context.Background(), "s1", "p", "m", "summary"); err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(r.histories["s1"]) <= before {
		t.Fatal("summary did not append to history")
	}

	// clear wipes, then leaves the marker exchange.
	if _, err := r.RunCommand(context.Background(), "s1", "p", "m", "clear"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	h = r.histories["s1"]
	if len(h) != 2 || h[0].Content != "/clear" {
		t.Fatalf("clear memory wrong: %+v", h)
	}

	// Unknown command errors.
	if _, err := r.RunCommand(context.Background(), "s1", "p", "m", "/nope"); err == nil {
		t.Fatal("unknown command must error")
	}

	// Persistence: every command exchange reached the sink.
	if len(sink.turns) != 4 {
		t.Fatalf("sink turns = %d, want 4", len(sink.turns))
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := estimateTokens("hello world, this is ascii"); got < 3 {
		t.Fatalf("ascii estimate too low: %d", got)
	}
	if got := estimateTokens("这是一段中文内容用于估算"); got < 5 {
		t.Fatalf("cjk estimate too low: %d", got)
	}
	if got := estimateTokens(""); got != 0 {
		t.Fatalf("empty should be 0, got %d", got)
	}
}

type llmResolverFunc func(providerID, model string) (domain.LLMEndpoint, error)

func (f llmResolverFunc) ResolveLLM(providerID, model string) (domain.LLMEndpoint, error) {
	return f(providerID, model)
}

type memTurnSink struct{ turns [][2]string }

func (m *memTurnSink) RecordTurn(sessionID, user, assistant string) {
	m.turns = append(m.turns, [2]string{user, assistant})
}
