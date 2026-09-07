package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/ai-remote/workspace/internal/domain"
)

// newMsgStream builds a StreamReader that yields msgs then EOF.
func newMsgStream(msgs []*schema.Message) *schema.StreamReader[*schema.Message] {
	sr, sw := schema.Pipe[*schema.Message](len(msgs))
	go func() {
		for _, m := range msgs {
			sw.Send(m, nil)
		}
		sw.Close()
	}()
	return sr
}

// The checker must route to the tools node whenever tool calls appear — no
// matter how much narration precedes them (the old ~200-byte cutoff ended
// turns early on Chinese preambles).
func TestHybridCheckerRoutesToolCallsAfterLongText(t *testing.T) {
	longText := ""
	for i := 0; i < 40; i++ {
		longText += "我继续检查WSL2特有的GPU路径和NVIDIA容器运行时：" // ~24 runes ≈ 72 bytes each
	}
	msgs := []*schema.Message{
		schema.AssistantMessage(longText, nil),
		schema.AssistantMessage("", []schema.ToolCall{{ID: "t1", Function: schema.FunctionCall{Name: "ssh_exec", Arguments: `{"command":"k3d version"}`}}}),
	}
	got, err := hybridToolCallChecker(t.Context(), newMsgStream(msgs))
	if err != nil {
		t.Fatalf("checker error: %v", err)
	}
	if !got {
		t.Fatalf("long preamble + tool call must route to tools node, got END")
	}
}

// Pure text (however long) with no tool calls routes to END — the final
// answer path.
func TestHybridCheckerRoutesPureTextToEnd(t *testing.T) {
	msgs := []*schema.Message{
		schema.AssistantMessage("诊断完成：CPU 占用正常。", nil),
		schema.AssistantMessage("内存与磁盘亦无异常。", nil),
	}
	got, err := hybridToolCallChecker(t.Context(), newMsgStream(msgs))
	if err != nil {
		t.Fatalf("checker error: %v", err)
	}
	if got {
		t.Fatalf("text-only stream must route to END")
	}
}

// Tool calls in the very first chunk route instantly (OpenAI-style).
func TestHybridCheckerRoutesImmediateToolCall(t *testing.T) {
	msgs := []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{ID: "t1", Function: schema.FunctionCall{Name: "ssh_exec", Arguments: `{}`}}}),
	}
	got, err := hybridToolCallChecker(t.Context(), newMsgStream(msgs))
	if err != nil {
		t.Fatalf("checker error: %v", err)
	}
	if !got {
		t.Fatalf("immediate tool call must route to tools node")
	}
}

// An empty stream (EOF right away) is a valid END.
func TestHybridCheckerEmptyStream(t *testing.T) {
	got, err := hybridToolCallChecker(t.Context(), newMsgStream(nil))
	if err != nil && err != io.EOF {
		t.Fatalf("checker error: %v", err)
	}
	if got {
		t.Fatalf("empty stream must route to END")
	}
}

// ── /skill and @path message resolution ─────────────────────────────────

type fakeSkills struct{}

func (fakeSkills) ListSkills() ([]domain.Skill, error) { return nil, nil }

func (fakeSkills) GetSkill(name string) (domain.Skill, error) {
	if name == "deploy" {
		return domain.Skill{Name: name, Content: "DEPLOY STEPS"}, nil
	}
	return domain.Skill{}, fmt.Errorf("skill %q not found", name)
}

func TestResolveUserMessage(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notes, []byte("hello-notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runtime{skills: fakeSkills{}}

	// Leading /name injects the skill body inline (eino inline mode).
	got := r.resolveUserMessage("sess-1", "/deploy rollout the api")
	if !strings.HasPrefix(got, "DEPLOY STEPS") || !strings.HasSuffix(got, "rollout the api") {
		t.Fatalf("skill injection failed: %q", got)
	}

	// Unknown skill: text passes through untouched.
	if got := r.resolveUserMessage("sess-1", "/nope do things"); got != "/nope do things" {
		t.Fatalf("unknown skill mutated: %q", got)
	}

	// @path on a local session loads the file content into a <file> block.
	got = r.resolveUserMessage("local-1", "check @"+filepath.ToSlash(notes)+" please")
	if !strings.Contains(got, "hello-notes") || !strings.Contains(got, `<file path=`) {
		t.Fatalf("file mention failed: %q", got)
	}

	// Unknown path: token stays visible to the model.
	if got := r.resolveUserMessage("local-1", "check @/no/such/file.log"); !strings.Contains(got, "@/no/such/file.log") {
		t.Fatalf("unknown path mutated: %q", got)
	}

	// No skills source: /mention is plain text.
	raw := &Runtime{}
	if got := raw.resolveUserMessage("sess-1", "/deploy x"); got != "/deploy x" {
		t.Fatalf("nil skills mutated message: %q", got)
	}
}

func TestCapContext(t *testing.T) {
	if got := capContext([]byte("short"), 1024); got != "short" {
		t.Fatalf("small content mutated: %q", got)
	}
	got := capContext([]byte(strings.Repeat("x", 300)), 100)
	if len(got) <= 100 || !strings.HasSuffix(got, "[truncated]") {
		t.Fatalf("cap/truncation mark missing")
	}
}

// ── diagnosis mode ────────────────────────────────────────────────────────

type fakeSnapshots struct{ out string }

func (f fakeSnapshots) Snapshot(_ context.Context, _ string) (string, error) {
	return f.out, nil
}

func TestComposeDiagnosisMessage(t *testing.T) {
	got := composeDiagnosisMessage("CPU 很高", "CPU: 90%")
	if !strings.HasPrefix(got, "CPU 很高") {
		t.Fatalf("symptom must lead: %q", got)
	}
	if !strings.Contains(got, "<health-snapshot>\nCPU: 90%\n</health-snapshot>") {
		t.Fatalf("snapshot block missing: %q", got)
	}

	// Without a snapshot the model still gets an explicit note.
	plain := composeDiagnosisMessage("s", "")
	if !strings.Contains(plain, "no health snapshot available") {
		t.Fatalf("missing-snapshot note absent: %q", plain)
	}

	// Oversized snapshots are truncated, with the marker kept.
	huge := composeDiagnosisMessage("s", strings.Repeat("x", snapshotCharBudget+500))
	if len(huge) > snapshotCharBudget+200 || !strings.Contains(huge, "[truncated]") {
		t.Fatalf("oversized snapshot not truncated (%d chars)", len(huge))
	}
}

// SetDiagnosisMode switches the system prompt to the triage template; it is
// keyed per session and cleared together with the history.
func TestDiagnosisPromptSwitch(t *testing.T) {
	r := &Runtime{diagnosis: map[string]bool{}}

	if r.inDiagnosisMode("sess-1") {
		t.Fatal("diagnosis mode must default off")
	}
	normal := r.systemPrompt("local-1")
	if strings.Contains(normal, "site-reliability diagnostician") {
		t.Fatal("normal prompt leaked the diagnosis template")
	}

	r.SetDiagnosisMode("local-1", true)
	diag := r.systemPrompt("local-1")
	if !strings.Contains(diag, "site-reliability diagnostician") ||
		!strings.Contains(diag, "现象 / Phenomenon") ||
		!strings.Contains(diag, "<health-snapshot>") {
		t.Fatalf("diagnosis prompt incomplete:\n%s", diag)
	}

	// Other sessions are unaffected.
	if r.inDiagnosisMode("sess-2") {
		t.Fatal("diagnosis mode leaked across sessions")
	}

	r.ClearHistory("local-1")
	if r.inDiagnosisMode("local-1") {
		t.Fatal("clear history must reset diagnosis mode")
	}
}

// ClearHistory keeps other sessions' histories intact.
func TestClearHistoryIsolation(t *testing.T) {
	r := &Runtime{histories: map[string][]*schema.Message{}, diagnosis: map[string]bool{}}
	r.recordTurn("a", "q", "ans-a")
	r.recordTurn("b", "q", "ans-b")
	r.ClearHistory("a")
	if len(r.histories["b"]) != 2 {
		t.Fatalf("session b history lost: %v", r.histories["b"])
	}
	if len(r.histories["a"]) != 0 {
		t.Fatalf("session a history not cleared")
	}
}
