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

// ── digital-employee experts (persona layer + snapshot window) ───────────

type fakeSnapshots struct{ out string }

func (f fakeSnapshots) Snapshot(_ context.Context, _ string) (string, error) {
	return f.out, nil
}

type fakeExperts struct{}

func (fakeExperts) GetExpert(id string) (domain.Expert, error) {
	if id == domain.ExpertIDDiagnosticsSRE {
		return domain.Expert{
			ID:           id,
			Name:         "SRE 诊断专家",
			Role:         "SRE",
			Description:  "evidence first",
			SystemPrompt: "You are an AI site-reliability diagnostician. 现象 / Phenomenon is part of the output contract.",
			AutoSnapshot: true,
			SkillRefs:    []string{"cpu-high"},
		}, nil
	}
	if id == "builtin-k8s-ops" {
		return domain.Expert{
			ID:           id,
			Name:         "K8s 运维专家",
			SystemPrompt: "KUBERNETES OPERATOR PERSONA",
			AllowedTools: []string{"ssh_exec", "skill"},
		}, nil
	}
	return domain.Expert{}, fmt.Errorf("expert %q not found", id)
}

func TestComposeSnapshotMessage(t *testing.T) {
	got := composeSnapshotMessage("CPU 很高", "CPU: 90%")
	if !strings.HasPrefix(got, "CPU 很高") {
		t.Fatalf("user message must lead: %q", got)
	}
	if !strings.Contains(got, "<health-snapshot>\nCPU: 90%\n</health-snapshot>") {
		t.Fatalf("snapshot block missing: %q", got)
	}

	// Without a snapshot the model still gets an explicit note.
	plain := composeSnapshotMessage("s", "")
	if !strings.Contains(plain, "no health snapshot available") {
		t.Fatalf("missing-snapshot note absent: %q", plain)
	}

	// Oversized snapshots are truncated, with the marker kept.
	huge := composeSnapshotMessage("s", strings.Repeat("x", snapshotCharBudget+500))
	if len(huge) > snapshotCharBudget+200 || !strings.Contains(huge, "[truncated]") {
		t.Fatalf("oversized snapshot not truncated (%d chars)", len(huge))
	}
}

// The SRE diagnostician persona carries the triage prompt and the fixed
// contracts; without an expert the default template is used.
func TestExpertPromptSwitch(t *testing.T) {
	r := &Runtime{experts: fakeExperts{}, activeExperts: map[string]string{}, snapshotDone: map[string]bool{}}

	if got := r.ExpertOf("local-1"); got != "" {
		t.Fatalf("expert must default empty, got %q", got)
	}
	normal := r.systemPrompt("local-1", nil, nil)
	if strings.Contains(normal, "site-reliability diagnostician") {
		t.Fatal("default prompt leaked a persona")
	}
	if !strings.Contains(normal, permissionLocalText) {
		t.Fatal("default prompt lost the permission contract")
	}

	exp, _ := r.resolveExpert(t.Context(), "local-1", domain.ExpertIDDiagnosticsSRE, "CPU 很高", nil)
	if exp == nil {
		t.Fatal("diagnosis expert not resolved")
	}
	diag := r.systemPrompt("local-1", exp, nil)
	if !strings.Contains(diag, "site-reliability diagnostician") ||
		!strings.Contains(diag, "现象 / Phenomenon") {
		t.Fatalf("expert persona missing from prompt:\n%s", diag)
	}
	// The fixed contracts must survive any persona.
	if !strings.Contains(diag, permissionLocalText) || !strings.Contains(diag, "local_exec(command)") {
		t.Fatalf("expert prompt lost the fixed contracts:\n%s", diag)
	}

	// Other sessions are unaffected.
	if got := r.ExpertOf("sess-2"); got != "" {
		t.Fatalf("expert leaked across sessions: %q", got)
	}

	// New conversation (ClearHistory) KEEPS the persona.
	r.ClearHistory("local-1")
	if got := r.ExpertOf("local-1"); got != domain.ExpertIDDiagnosticsSRE {
		t.Fatalf("clear history must keep the expert, got %q", got)
	}
}

// An AutoSnapshot expert injects the health snapshot on its activation turn
// only; the window reopens on expert switch and on ClearHistory.
func TestExpertSnapshotWindow(t *testing.T) {
	r := &Runtime{
		experts:       fakeExperts{},
		snapshots:     fakeSnapshots{out: "CPU: 90%"},
		activeExperts: map[string]string{},
		snapshotDone:  map[string]bool{},
	}
	ctx := t.Context()

	// First turn with the diagnosis expert: snapshot injected.
	_, msg := r.resolveExpert(ctx, "s1", domain.ExpertIDDiagnosticsSRE, "CPU 很高", nil)
	if !strings.Contains(msg, "<health-snapshot>") {
		t.Fatalf("activation turn must inject the snapshot: %q", msg)
	}

	// Follow-up turn: no fresh snapshot.
	_, msg = r.resolveExpert(ctx, "s1", domain.ExpertIDDiagnosticsSRE, "还是很高", nil)
	if strings.Contains(msg, "<health-snapshot>") {
		t.Fatalf("follow-up turn must not re-inject: %q", msg)
	}

	// Switching experts (even away and back) reopens the window.
	r.resolveExpert(ctx, "s1", "builtin-k8s-ops", "看下节点", nil)
	_, msg = r.resolveExpert(ctx, "s1", domain.ExpertIDDiagnosticsSRE, "继续", nil)
	if !strings.Contains(msg, "<health-snapshot>") {
		t.Fatalf("switch back must re-inject: %q", msg)
	}

	// ClearHistory reopens it too (new conversation = fresh triage).
	r.ClearHistory("s1")
	_, msg = r.resolveExpert(ctx, "s1", domain.ExpertIDDiagnosticsSRE, "新话题", nil)
	if !strings.Contains(msg, "<health-snapshot>") {
		t.Fatalf("new conversation must re-inject: %q", msg)
	}

	// Unknown expert ids degrade to the general assistant.
	exp, msg := r.resolveExpert(ctx, "s2", "no-such-expert", "hello", nil)
	if exp != nil || strings.Contains(msg, "<health-snapshot>") {
		t.Fatalf("unknown expert must degrade silently: %v %q", exp, msg)
	}
}

// An expert's tool allowlist scopes the prompt's tool contract and (via the
// toolset filter) the model's actual toolset; the permission contract stays.
func TestExpertToolAllowlist(t *testing.T) {
	r := &Runtime{experts: fakeExperts{}, activeExperts: map[string]string{}, snapshotDone: map[string]bool{}}

	allowed := map[string]bool{"ssh_exec": true, "skill": true}
	prompt := r.systemPrompt("sess-1", &domain.Expert{Name: "K8s 运维专家", SystemPrompt: "P", AllowedTools: []string{"ssh_exec", "skill"}}, allowed)
	if !strings.Contains(prompt, "ssh_exec(command)") {
		t.Fatal("allowed tool missing from contract")
	}
	if strings.Contains(prompt, "upload(localPath") {
		t.Fatal("filtered tool leaked into the contract")
	}
	if !strings.Contains(prompt, permissionRemoteText) {
		t.Fatal("allowlisting dropped the permission contract")
	}

	// Empty allowlist = everything.
	full := r.systemPrompt("sess-1", &domain.Expert{Name: "X"}, nil)
	for _, want := range []string{"ssh_exec", "upload", "download"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full contract lost %s", want)
		}
	}
}

// ClearHistory keeps other sessions' histories intact.
func TestClearHistoryIsolation(t *testing.T) {
	r := &Runtime{histories: map[string][]*schema.Message{}, snapshotDone: map[string]bool{}}
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
