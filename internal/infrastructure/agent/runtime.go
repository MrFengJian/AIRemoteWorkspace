// Package agent implements the AI Agent runtime: it constructs eino ReAct
// agents per SSH session, streams their output, and routes tool calls through
// the permission gate.
//
// It lives in infrastructure (not application) because it depends on eino,
// ssh, sftp, and the tools package — all infrastructure concerns. The
// application layer defines the port interfaces (AgentRuntime, AgentEvents).
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/agent/tools"
	"github.com/ai-remote/workspace/internal/infrastructure/sftp"
	"github.com/ai-remote/workspace/internal/infrastructure/ssh"
)

// SkillSource loads agent skills (skill directories) — implemented by the
// application layer's SkillService, or by an expert-scoped view of it. May be
// nil: /skill resolution and the model-facing skill tool are then disabled.
type SkillSource = domain.SkillStore

// expertSkillScoper narrows a skill source to one expert's view (private
// packs shadowing same-name public packs). Implemented by SkillService when
// an experts root is wired.
type expertSkillScoper interface {
	SkillSourceFor(expertID string) (domain.SkillStore, bool)
}

// ExpertSource resolves digital-employee expert personas by id — implemented
// by the application layer's ExpertService. May be nil: expert ids then
// degrade to the general assistant.
type ExpertSource interface {
	GetExpert(id string) (domain.Expert, error)
}

// SnapshotSource collects the deterministic health-check snapshot for a
// session's host (application layer's MonitorService). May be nil: diagnosis
// chats then start without triage context instead of failing.
type SnapshotSource interface {
	Snapshot(ctx context.Context, sessionID string) (string, error)
}

// AgentEvents delivers streaming agent output upward (to the Wails service).
// A tool invocation is reported twice with the same stepID: once when it
// starts (OnToolCallStart) and once when the result is available
// (OnToolCallEnd) — the UI folds both into one step. Both come from the
// tool-layer observer (tools.RunObserver), which works for every model.
type AgentEvents interface {
	OnChunk(sessionID, text string)
	OnToolCallStart(sessionID, callID, toolName, args string)
	OnToolCallEnd(sessionID, callID, result string)
	OnDone(sessionID string)
	OnError(sessionID, errMsg string)
}

// eventsObserver adapts AgentEvents to tools.RunObserver.
type eventsObserver struct{ events AgentEvents }

func (o eventsObserver) OnToolStart(sessionID, stepID, toolName, args string) {
	if o.events != nil {
		o.events.OnToolCallStart(sessionID, stepID, toolName, args)
	}
}

func (o eventsObserver) OnToolEnd(sessionID, stepID, result string) {
	if o.events != nil {
		o.events.OnToolCallEnd(sessionID, stepID, result)
	}
}

// LLMResolver resolves a (providerID, model) selection into endpoint
// credentials for building the chat model. Implemented by the application
// layer's ModelProviderService.
type LLMResolver interface {
	ResolveLLM(providerID, model string) (domain.LLMEndpoint, error)
}

// TurnSink receives each completed conversation turn (persistence decoupled
// from memory). Implemented by the application layer's ConversationService;
// nil is allowed (no persistence).
type TurnSink interface {
	RecordTurn(sessionID, user, assistant string)
}

// CredsResolver returns host + credentials for a session (for SFTP tools).
type CredsResolver = tools.CredsResolver

// PermissionGate is the approval gate tools call before WRITE/DANGEROUS ops.
type PermissionGate = tools.PermissionGate

// SftpFileOps is the subset of the SFTP manager the file tools need.
type SftpFileOps = tools.SftpFileOps

// Conversation-memory budget: replayed history is capped by characters
// (≈6k tokens) and turn count, whichever binds first. Only complete turns
// (user + final assistant answer) are recorded; tool steps stay internal to
// their turn — the final answer summarizes them.
const (
	historyCharBudget = 24 * 1024
	maxHistoryTurns   = 40
)

// Runtime manages per-session ReAct agents and streams their output.
// Conversation history is kept in memory per session and replayed on each
// turn (multi-turn memory); ClearHistory drops it.
type Runtime struct {
	llm       LLMResolver
	sshMgr    *ssh.Manager
	sftp      SftpFileOps
	gate      PermissionGate
	secrets   SecretsForResolver
	sink      TurnSink
	agentCfg  AgentConfigSource
	skills    SkillSource
	snapshots SnapshotSource
	experts   ExpertSource

	mu            sync.Mutex
	cancelFns     map[string]context.CancelFunc
	histories     map[string][]*schema.Message
	activeExperts map[string]string // sessionID → expert id resolved on the last turn
	snapshotDone  map[string]bool   // sessionID → snapshot injected for the current expert window
}

// SecretsForResolver provides remembered host secrets for credential resolution.
type SecretsForResolver interface {
	GetHostSecret(hostID string, kind string) ([]byte, error)
}

// AgentConfigSource supplies the current agent tunables from the global
// settings (application layer's ConfigService). Read per chat so setting
// changes apply to the next turn without a restart. May be nil — the
// built-in defaults are used then.
type AgentConfigSource func() domain.AgentConfig

// NewRuntime wires the agent runtime. sink (may be nil) persists completed
// turns — the application layer's ConversationService. skills (may be nil)
// enables /skill invocation and the model-facing skill tool. snapshots (may
// be nil) disables the AutoSnapshot experts' health-check snapshot. experts
// (may be nil) disables the digital-employee persona layer.
func NewRuntime(llm LLMResolver, sshMgr *ssh.Manager, sftp SftpFileOps, gate PermissionGate, secrets SecretsForResolver, sink TurnSink, agentCfg AgentConfigSource, skills SkillSource, snapshots SnapshotSource, experts ExpertSource) *Runtime {
	return &Runtime{
		llm:           llm,
		sshMgr:        sshMgr,
		sftp:          sftp,
		gate:          gate,
		secrets:       secrets,
		sink:          sink,
		agentCfg:      agentCfg,
		skills:        skills,
		snapshots:     snapshots,
		experts:       experts,
		cancelFns:     make(map[string]context.CancelFunc),
		histories:     make(map[string][]*schema.Message),
		activeExperts: make(map[string]string),
		snapshotDone:  make(map[string]bool),
	}
}

// agentConfig returns the effective settings with defaults for zero fields
// (a nil source or a zero block in an older settings row).
func (r *Runtime) agentConfig() domain.AgentConfig {
	cfg := domain.AgentConfig{}
	if r.agentCfg != nil {
		cfg = r.agentCfg()
	}
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 100
	}
	if cfg.HistoryTurns <= 0 {
		cfg.HistoryTurns = maxHistoryTurns
	}
	if cfg.ToolOutputLimitKB <= 0 {
		cfg.ToolOutputLimitKB = 64
	}
	return cfg
}

// Chat starts a streaming agent chat for a session using the selected
// provider + model. expertID ("" = the general assistant) selects the
// digital-employee persona for the turn; the session's conversation history
// is replayed so the model keeps context across turns.
func (r *Runtime) Chat(ctx context.Context, sessionID, providerID, model, expertID, userMessage string, events AgentEvents) error {
	return r.runChat(ctx, sessionID, providerID, model, expertID, userMessage, userMessage, events)
}

// SetExpert records a session's active expert without starting a chat (used
// on resume and on explicit switches). An AutoSnapshot expert's snapshot
// window opens here: its next chat turn carries a fresh health snapshot.
func (r *Runtime) SetExpert(sessionID, expertID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeExperts[sessionID] != expertID {
		delete(r.snapshotDone, sessionID)
	}
	r.activeExperts[sessionID] = expertID
}

// ExpertOf returns the session's currently recorded expert id ("" = none).
func (r *Runtime) ExpertOf(sessionID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeExperts[sessionID]
}

// snapshotTimeout bounds the deterministic health-check round: overview and
// process collection each sleep 1s between samples, plus transfer overhead.
const snapshotTimeout = 30 * time.Second

// snapshotUnavailableNote is the user-visible pre-turn notice and the model
// message note when snapshot collection fails.
func snapshotUnavailableNote(err error) string {
	return fmt.Sprintf("体检快照不可用（%v），请直接通过只读命令采集所需上下文。/ Health snapshot unavailable (%v); gather context via read-only commands instead.", err, err)
}

// composeSnapshotMessage builds the model-facing first turn: the user's
// message plus the health snapshot as a structured context block.
func composeSnapshotMessage(user, snapshot string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(user))
	if snapshot != "" {
		if len(snapshot) > snapshotCharBudget {
			snapshot = snapshot[:snapshotCharBudget] + "\n…[truncated]"
		}
		b.WriteString("\n\n<health-snapshot>\n")
		b.WriteString(snapshot)
		b.WriteString("\n</health-snapshot>")
	} else {
		b.WriteString("\n\n(no health snapshot available — start from read-only evidence gathering)")
	}
	return b.String()
}

// snapshotCharBudget bounds the injected snapshot (≈2k tokens).
const snapshotCharBudget = 8 * 1024

// skillStoreFor returns the skill view for this turn: an expert-scoped store
// (private packs shadow same-name public packs) when the expert has private
// skills, the global store otherwise.
func (r *Runtime) skillStoreFor(exp *domain.Expert) domain.SkillStore {
	if r.skills == nil || exp == nil {
		return r.skills
	}
	if scoper, ok := r.skills.(expertSkillScoper); ok {
		if store, scoped := scoper.SkillSourceFor(exp.ID); scoped {
			return store
		}
	}
	return r.skills
}

// resolveExpert returns the effective expert for this turn (the general
// assistant is the default expert — an empty or unknown id resolves to it
// when wired) and the model-facing user message. Recording the expert per
// session also detects persona switches: whenever the expert changes — or a
// new conversation begins — the snapshot window reopens, so an AutoSnapshot
// expert's next turn carries a fresh health snapshot (injected into the model
// message; a failed collection degrades to a pre-turn notice, never an error).
func (r *Runtime) resolveExpert(ctx context.Context, sessionID, expertID, modelUser string, events AgentEvents) (*domain.Expert, string) {
	var exp *domain.Expert
	if r.experts != nil {
		// The general assistant is a real (default) expert now: an empty id —
		// and an unknown one — resolve to it, so its welcome card, suggested
		// prompts and persona apply without an explicit selection.
		lookup := expertID
		if lookup == "" {
			lookup = domain.ExpertIDGeneralAssistant
		}
		if e, err := r.experts.GetExpert(lookup); err == nil && e.Enabled && !e.Dismissed {
			exp = &e
		} else {
			// Unknown/disabled id: degrade to the default expert when it is
			// available, no persona otherwise.
			if e, err := r.experts.GetExpert(domain.ExpertIDGeneralAssistant); err == nil && e.Enabled && !e.Dismissed {
				exp = &e
			}
		}
	}

	r.mu.Lock()
	if r.activeExperts[sessionID] != expertID {
		r.activeExperts[sessionID] = expertID
		delete(r.snapshotDone, sessionID)
	}
	inject := exp != nil && exp.AutoSnapshot && !r.snapshotDone[sessionID]
	if inject {
		r.snapshotDone[sessionID] = true
	}
	r.mu.Unlock()

	if inject {
		snapshot := ""
		if r.snapshots != nil {
			sctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
			snap, snapErr := r.snapshots.Snapshot(sctx, sessionID)
			cancel()
			if snapErr != nil {
				if events != nil {
					// Pre-turn notice: the model message will also carry the note.
					events.OnChunk(sessionID, fmt.Sprintf("> %s\n\n", snapshotUnavailableNote(snapErr)))
				}
			} else {
				snapshot = snap
			}
		}
		modelUser = composeSnapshotMessage(modelUser, snapshot)
	}
	return exp, modelUser
}

// runChat is the shared streaming pipeline: rawUser is what the conversation
// memory records; modelUser is what the model actually receives (context
// expansion already applied).
func (r *Runtime) runChat(ctx context.Context, sessionID, providerID, model, expertID, rawUser, modelUser string, events AgentEvents) error {
	exp, modelUser := r.resolveExpert(ctx, sessionID, expertID, modelUser, events)

	ep, err := r.llm.ResolveLLM(providerID, model)
	if err != nil {
		return err
	}
	// Local OpenAI-compatible endpoints (Ollama, LM Studio) need no real key,
	// but the client requires a non-empty one.
	apiKey := ep.APIKey
	if apiKey == "" {
		apiKey = "local-no-key"
	}

	chatModelCfg := &openaimodel.ChatModelConfig{
		BaseURL: ep.BaseURL,
		APIKey:  apiKey,
		Model:   ep.Model,
	}
	// Per-expert sampling: a persona with a temperature preference gets it,
	// everyone else uses the model default.
	if exp != nil && exp.Temperature > 0 {
		t := float32(exp.Temperature)
		chatModelCfg.Temperature = &t
	}
	chatModel, err := openaimodel.NewChatModel(ctx, chatModelCfg)
	if err != nil {
		return fmt.Errorf("create chat model: %w", err)
	}

	credsResolver := r.buildResolver()
	cfg := r.agentConfig()
	if exp != nil && exp.MaxSteps > 0 {
		cfg.MaxSteps = exp.MaxSteps
	}
	// The expert's tool allowlist scopes the toolset; nil/empty = all.
	var allowed map[string]bool
	if exp != nil && len(exp.AllowedTools) > 0 {
		allowed = make(map[string]bool, len(exp.AllowedTools))
		for _, name := range exp.AllowedTools {
			allowed[name] = true
		}
	}
	// Expert-scoped skill view: the expert's private packs shadow same-name
	// public packs for this session's skill tool and bound-skills listing.
	skillStore := r.skillStoreFor(exp)
	ts, err := tools.NewToolSet(tools.Deps{
		SSH:              r.sshMgr,
		SFTP:             r.sftp,
		OutputLimitBytes: cfg.ToolOutputLimitKB * 1024,
		Skills:           skillStore,
	}, credsResolver, r.gate, eventsObserver{events})
	if err != nil {
		return fmt.Errorf("build toolset: %w", err)
	}
	// Local terminal sessions (id prefix "local-") have no SSH host behind
	// them: expose only the local tools.
	var toolList []tool.BaseTool
	if strings.HasPrefix(sessionID, "local-") {
		toolList, err = ts.BuildLocalForSession(sessionID, allowed)
	} else {
		toolList, err = ts.BuildForSession(sessionID, allowed)
	}
	if err != nil {
		return fmt.Errorf("build session tools: %w", err)
	}

	ag, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: toolList,
		},
		MaxStep: cfg.MaxSteps,
		// Route the model's stream: tool calls anywhere → tools node; pure
		// text past a short preamble → END. The default first-chunk checker
		// breaks models that emit text before tool calls (Claude, DeepSeek-R1).
		StreamToolCallChecker: hybridToolCallChecker,
	})
	if err != nil {
		return fmt.Errorf("create react agent: %w", err)
	}

	chatCtx, cancel := context.WithCancel(ctx)
	r.registerCancel(sessionID, cancel)
	defer r.unregisterCancel(sessionID)

	// [system] + replayed history + this turn's user message. The message is
	// resolved (/skill → instructions, @path → file content) for the model
	// only — the raw text is what gets recorded into conversation memory.
	// The /skill resolution uses the same expert-scoped store as the tool.
	msgs := make([]*schema.Message, 0, 8)
	msgs = append(msgs, schema.SystemMessage(r.systemPrompt(sessionID, exp, allowed)))
	r.mu.Lock()
	msgs = append(msgs, r.histories[sessionID]...)
	r.mu.Unlock()
	msgs = append(msgs, schema.UserMessage(r.resolveUserMessage(sessionID, modelUser, skillStore)))

	reader, err := ag.Stream(chatCtx, msgs)
	if err != nil {
		return fmt.Errorf("agent stream: %w", err)
	}
	defer reader.Close()

	// Tool steps are reported by the tool-layer observer (see eventsObserver);
	// this loop only carries the final assistant text.
	var finalText strings.Builder
	for {
		msg, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, context.Canceled) && events != nil {
				events.OnError(sessionID, "cancelled")
				return err
			}
			if events != nil {
				events.OnError(sessionID, err.Error())
			}
			// Keep whatever the model produced before the failure in the
			// conversation memory, so a follow-up "继续" has context instead
			// of a gap (tool results live only inside the aborted eino run,
			// but the partial narration still anchors the continuation).
			if finalText.Len() > 0 {
				r.recordTurn(sessionID, rawUser, finalText.String())
			}
			return err
		}
		if msg.Content != "" {
			if events != nil {
				events.OnChunk(sessionID, msg.Content)
			}
			finalText.WriteString(msg.Content)
		}
	}

	if events != nil {
		events.OnDone(sessionID)
	}
	// Record the completed turn for multi-turn memory. Failed/cancelled turns
	// are not recorded (the user saw no complete answer).
	if finalText.Len() > 0 {
		r.recordTurn(sessionID, rawUser, finalText.String())
	}
	return nil
}

// Cancel cancels an ongoing chat.
func (r *Runtime) Cancel(sessionID string) {
	r.mu.Lock()
	cancel, ok := r.cancelFns[sessionID]
	r.mu.Unlock()
	if ok {
		cancel()
	}
}

// ClearHistory forgets a session's conversation (frontend "clear chat") and
// reopens the current expert's snapshot window — the next AutoSnapshot turn
// carries a fresh health snapshot. The expert selection itself is KEPT: a
// new conversation is a new topic with the same digital employee; leaving
// the persona is an explicit switch (badge X / selector).
func (r *Runtime) ClearHistory(sessionID string) {
	r.mu.Lock()
	delete(r.histories, sessionID)
	delete(r.snapshotDone, sessionID)
	r.mu.Unlock()
}

// RestoreHistory replaces a session's conversation memory with persisted
// messages (resuming a conversation). Pairs are expected to be user/assistant
// turns; they are trimmed to the same budget as live recording.
func (r *Runtime) RestoreHistory(sessionID string, msgs []*schema.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := append([]*schema.Message(nil), msgs...)
	total := 0
	for _, m := range h {
		total += len(m.Content)
	}
	turns := r.agentConfig().HistoryTurns
	i := 0
	for i+2 <= len(h)-2 && (total > historyCharBudget || (len(h)-i)/2 > turns) {
		total -= len(h[i].Content) + len(h[i+1].Content)
		i += 2
	}
	r.histories[sessionID] = h[i:]
}

// recordTurn appends a completed (user, assistant) turn and trims the oldest
// turns to stay within the char/turn budget.
func (r *Runtime) recordTurn(sessionID, user, assistant string) {
	r.mu.Lock()
	h := append(r.histories[sessionID],
		schema.UserMessage(user),
		schema.AssistantMessage(assistant, nil))

	total := len(user) + len(assistant)
	turns := r.agentConfig().HistoryTurns
	i := 0
	// History is (user, assistant) pairs; drop two at a time. Always keep the
	// newest turn (the last two messages).
	for i+2 <= len(h)-2 && (total > historyCharBudget || (len(h)-i)/2 > turns) {
		total -= len(h[i].Content) + len(h[i+1].Content)
		i += 2
	}
	r.histories[sessionID] = h[i:]
	r.mu.Unlock()

	// Persist after unlocking (the sink hits the DB).
	if r.sink != nil {
		r.sink.RecordTurn(sessionID, user, assistant)
	}
}

func (r *Runtime) registerCancel(sessionID string, cancel context.CancelFunc) {
	r.mu.Lock()
	r.cancelFns[sessionID] = cancel
	r.mu.Unlock()
}

func (r *Runtime) unregisterCancel(sessionID string) {
	r.mu.Lock()
	delete(r.cancelFns, sessionID)
	r.mu.Unlock()
}

// ── Message context resolution (/skill and @path mentions) ─────────────

var mentionRe = regexp.MustCompile(`@[^\s]+`)

// resolveUserMessage expands the input-box shortcuts before the message
// reaches the model:
//   - a leading `$name` loads the skill's SKILL.md instructions inline
//     (eino skill middleware's inline mode) and prepends them to the text;
//   - `@/some/path` tokens are replaced by <file path="…"> blocks holding
//     the file's content, loaded over SFTP (remote session) or from disk
//     (local session).
//
// Unresolvable mentions are left untouched so the model sees what the user
// typed. The recorded conversation history keeps the RAW message. The skill
// store is the turn's expert-scoped view (private packs shadow public ones).
func (r *Runtime) resolveUserMessage(sessionID, text string, skills domain.SkillStore) string {
	if skills != nil && strings.HasPrefix(text, "$") {
		rest := strings.TrimLeft(text[1:], " \t")
		name := rest
		remainder := ""
		if i := strings.IndexAny(rest, " \t\n\r"); i >= 0 {
			name = rest[:i]
			remainder = strings.TrimLeft(rest[i:], " \t")
		}
		if name != "" {
			if sk, err := skills.GetSkill(name); err == nil {
				text = strings.TrimSpace(sk.Content) + "\n\n---\n\n" + strings.TrimSpace(remainder)
			}
			// Unknown skill: leave the message exactly as typed.
		}
	}
	text = mentionRe.ReplaceAllStringFunc(text, func(tok string) string {
		path := strings.TrimPrefix(tok, "@")
		if path == "" {
			return tok
		}
		content, err := r.loadContextFile(sessionID, path)
		if err != nil {
			return tok // leave unknown paths visible to the model
		}
		return fmt.Sprintf("<file path=%q>\n%s\n</file>", path, content)
	})
	return text
}

// loadContextFile reads one @-mentioned file: from disk for local sessions,
// over SFTP for remote ones. Content is capped at the agent's tool output
// budget so a huge log cannot flood the context.
func (r *Runtime) loadContextFile(sessionID, path string) (string, error) {
	if strings.HasPrefix(sessionID, "local-") {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return capContext(data, r.agentConfig().ToolOutputLimitKB*1024), nil
	}
	if r.sftp == nil {
		return "", fmt.Errorf("sftp not available")
	}
	host, creds, err := r.buildResolver()(sessionID)
	if err != nil {
		return "", err
	}
	data, err := r.sftp.DownloadFile(host, creds, path, nil)
	if err != nil {
		return "", err
	}
	return capContext(data, r.agentConfig().ToolOutputLimitKB*1024), nil
}

// capContext bounds the injected content and marks the elision.
func capContext(data []byte, limit int) string {
	if limit <= 0 || len(data) <= limit {
		return string(data)
	}
	return string(data[:limit]) + "\n…[truncated]"
}

// PathEntry is one entry of an @-mention directory listing.
type PathEntry struct {
	Name  string
	IsDir bool
	Size  int64
}

// dirLister is the directory-listing capability of the concrete SFTP manager
// (the narrow SftpFileOps interface the runtime normally holds lacks it).
type dirLister interface {
	ListDir(host domain.Host, creds domain.Credentials, dir string) ([]sftp.Entry, error)
}

// ListContextPaths lists a directory for the @-completion popup: from disk
// for local sessions, over SFTP for remote ones.
func (r *Runtime) ListContextPaths(sessionID, dir string) ([]PathEntry, error) {
	if strings.HasPrefix(sessionID, "local-") {
		if dir == "" {
			dir = "."
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		out := make([]PathEntry, 0, len(entries))
		for _, e := range entries {
			size := int64(0)
			if !e.IsDir() {
				if info, err := e.Info(); err == nil {
					size = info.Size()
				}
			}
			out = append(out, PathEntry{Name: e.Name(), IsDir: e.IsDir(), Size: size})
		}
		return out, nil
	}
	lister, ok := r.sftp.(dirLister)
	if !ok {
		return nil, fmt.Errorf("directory listing not available")
	}
	host, creds, err := r.buildResolver()(sessionID)
	if err != nil {
		return nil, err
	}
	if dir == "" {
		dir = "/"
	}
	entries, err := lister.ListDir(host, creds, dir)
	if err != nil {
		return nil, err
	}
	out := make([]PathEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, PathEntry{Name: e.Name, IsDir: e.IsDir, Size: e.Size})
	}
	return out, nil
}

// hybridToolCallChecker routes the model's streamed output using only
// definitive signals:
//   - any chunk carrying tool calls → tools node;
//   - end of stream with no tool calls → END (it's the final answer).
//
// Earlier versions cut over to END after ~200 bytes of text — but models
// that narrate before calling tools (Claude, DeepSeek…, and any of them in
// Chinese, where 200 bytes ≈ 66 characters) regularly tripped it, ending the
// turn right after "I'll continue checking…" and silently dropping the tool
// calls. Deciding on EOF alone delays first-token delivery for pure-text
// answers, which is a cheap price for never misrouting an agentic turn.
//
// The checker receives its own fork of the stream (eino copies the branch
// input), so consuming it never loses data downstream.
func hybridToolCallChecker(_ context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
	defer sr.Close()
	for {
		msg, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(msg.ToolCalls) > 0 {
			return true, nil
		}
	}
}

// ── System prompt composition ──────────────────────────────────────────
//
// The prompt is layered: persona (expert or built-in default) + fixed
// contracts. The tool and permission texts are shared fragments so every
// variant — default or expert — states exactly the same tool semantics and
// approval rules. A persona can never override the permission contract: it
// is composed by the runtime after the persona text, every turn.

// toolDoc is one line of the tool contract: the tool names it covers (for
// expert allowlist filtering) and its description text.
type toolDoc struct {
	names []string
	text  string
}

// remoteToolDocs describes the remote-session toolset in default-prompt
// order. Grouped lines list every tool name the line covers.
var remoteToolDocs = []toolDoc{
	{[]string{"ssh_exec"}, "- ssh_exec(command): run a shell command on the remote host. Returns combined stdout+stderr; a non-zero exit is reported as [exit status N] — that is diagnostic output, not a failure."},
	{[]string{"ssh_read_file", "ssh_write_file"}, "- ssh_read_file(path) / ssh_write_file(path, content): read or overwrite a remote file over SFTP."},
	{[]string{"upload", "download"}, "- upload(localPath, remotePath) / download(remotePath, localPath): move files between the user's machine and the host."},
	{[]string{"local_exec", "local_read_file"}, "- local_exec(command) / local_read_file(path): run/read on the user's LOCAL machine. Prefer the remote tools unless local context is required."},
}

// localToolDocs describes the local-session toolset.
var localToolDocs = []toolDoc{
	{[]string{"local_exec"}, "- local_exec(command): run a shell command locally. Returns combined stdout+stderr; a non-zero exit is reported as [exit status N] — diagnostic output, not a failure."},
	{[]string{"local_read_file"}, "- local_read_file(path): read a local file as text."},
}

// toolContract renders the "Tools:" block for a session kind, filtered by an
// expert allowlist (nil/empty = every line). A grouped line stays when ANY of
// its tools is allowed — the runtime never grants a filtered-out tool, this
// only keeps the text honest about what the model can call.
func toolContract(isLocal bool, allowed map[string]bool) string {
	docs := remoteToolDocs
	if isLocal {
		docs = localToolDocs
	}
	lines := make([]string, 0, len(docs))
	for _, d := range docs {
		if len(allowed) == 0 {
			lines = append(lines, d.text)
			continue
		}
		for _, n := range d.names {
			if allowed[n] {
				lines = append(lines, d.text)
				break
			}
		}
	}
	return "Tools:\n" + strings.Join(lines, "\n")
}

const (
	remoteWorkflowText = "Workflow: start with read-only diagnostics (uptime, df -h, free -m, ps aux, journalctl …), analyze the output, " +
		"then summarize findings in concise markdown and propose fixes."

	remoteContainerText = "Container workloads: if the host runs Docker or Kubernetes, use the CLIs directly through ssh_exec " +
		"(docker ps / logs / stats / inspect, kubectl get/describe/logs) — they are the preferred interface for " +
		"container diagnostics. Prefer bounded output (docker logs --tail, kubectl logs --tail) to keep responses small."

	localContainerText = "Container workloads: if Docker Desktop or a local engine is installed, use the docker CLI through local_exec " +
		"(docker ps / logs / stats / inspect). Prefer bounded output (docker logs --tail) to keep responses small."

	// permissionRemoteText / permissionLocalText are the fixed approval
	// semantics — identical to what the built-in prompts always stated.
	permissionRemoteText = "Permissions: state-changing operations (file writes, uploads, package/service mutations, container lifecycle " +
		"control such as docker run/stop/restart, destructive commands) " +
		"are subject to the session's approval policy — the user may be asked to approve them. " +
		"If a tool result says the user DENIED the operation, do NOT retry it — " +
		"explain what you were about to do and propose an alternative."

	permissionLocalText = "Permissions: state-changing operations (file writes, package/service mutations, container lifecycle control " +
		"such as docker run/stop/restart, destructive commands) are subject to the session's approval policy — " +
		"the user may be asked to approve them. If a tool result says the user DENIED the operation, " +
		"do NOT retry it — explain and propose an alternative."
)

// systemPrompt is the LLM-facing contract: persona + environment + tool and
// approval semantics. exp (may be nil) selects the digital-employee persona;
// without one the built-in operations-assistant template is used. The user's
// standing instructions from global settings are appended to every variant.
func (r *Runtime) systemPrompt(sessionID string, exp *domain.Expert, allowed map[string]bool) string {
	isLocal := strings.HasPrefix(sessionID, "local-")
	var base string
	switch {
	case exp != nil:
		base = r.expertPrompt(exp, sessionID, isLocal, allowed)
	case isLocal:
		base = "You are an AI operations assistant working on the user's LOCAL machine (a local terminal session, no remote host).\n\n" +
			toolContract(true, nil) + "\n\n" +
			"Workflow: start with read-only diagnostics, analyze, then summarize findings in concise markdown and propose fixes.\n\n" +
			localContainerText + "\n\n" +
			permissionLocalText
	default:
		host, ok := r.sshMgr.HostOfSession(sessionID)
		name := "unknown"
		if ok {
			name = fmt.Sprintf("%s@%s", host.Username, host.Host)
		}
		base = fmt.Sprintf(
			"You are an AI operations assistant embedded in an SSH workspace, connected to host %s.\n\n"+
				toolContract(false, nil)+"\n\n"+
				remoteWorkflowText+"\n\n"+
				remoteContainerText+"\n\n"+
				permissionRemoteText,
			name,
		)
	}
	if custom := r.agentConfig().CustomInstructions; strings.TrimSpace(custom) != "" {
		base += "\n\n# The user's standing instructions (highest priority short of safety rules)\n" + custom
	}
	return base
}

// expertPrompt composes a digital employee's prompt: identity card, the
// persona's own instructions, the environment line, and the fixed tool and
// permission contracts (never trust a persona to state them), plus the
// expert's bound skills when a skill source is wired.
func (r *Runtime) expertPrompt(e *domain.Expert, sessionID string, isLocal bool, allowed map[string]bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %q", e.Name)
	if e.Role != "" {
		fmt.Fprintf(&b, " (%s)", e.Role)
	}
	b.WriteString(" — a digital-employee ops expert working inside the AI Remote Workspace.")
	if d := strings.TrimSpace(e.Description); d != "" {
		b.WriteString("\nMission: " + d)
	}
	if p := strings.TrimSpace(e.SystemPrompt); p != "" {
		b.WriteString("\n\n# Persona & working method\n" + p)
	}
	if h := strings.TrimSpace(e.Heartbeat); h != "" {
		b.WriteString("\n\n# Heartbeat — operational guidelines\n" + h +
			"\n\n(Heartbeat routines are proposals for the user to adopt — never run periodic loops " +
			"or recurring checks on your own initiative; every state-changing action still follows " +
			"the approval contract below.)")
	}

	b.WriteString("\n\n# Environment\n")
	if isLocal {
		b.WriteString("This is a LOCAL terminal session on the user's machine — no remote host is attached; " +
			"only the local tools are available.")
	} else if r.sshMgr == nil {
		b.WriteString("The session is connected to a remote host over SSH; the remote tools act on it.")
	} else if host, ok := r.sshMgr.HostOfSession(sessionID); ok {
		fmt.Fprintf(&b, "The session is connected to host %s@%s over SSH; the remote tools act on it.",
			host.Username, host.Host)
	} else {
		b.WriteString("The session is connected to a remote host over SSH; the remote tools act on it.")
	}

	b.WriteString("\n\n" + toolContract(isLocal, allowed) + "\n\n")
	if isLocal {
		b.WriteString(permissionLocalText)
	} else {
		b.WriteString(permissionRemoteText)
	}

	if store := r.skillStoreFor(e); store != nil && len(e.SkillRefs) > 0 {
		var lines []string
		for _, name := range e.SkillRefs {
			if sk, err := store.GetSkill(name); err == nil {
				lines = append(lines, fmt.Sprintf("- %s: %s", sk.Name, sk.Description))
			}
		}
		if len(lines) > 0 {
			b.WriteString("\n\n# Bound skills (load their full instructions with the `skill` tool when relevant)\n" +
				strings.Join(lines, "\n"))
		}
	}
	return b.String()
}

// transcriptCharBudget bounds the conversation transcript fed to the
// distillation call (≈10k tokens covers any persisted conversation).
const transcriptCharBudget = 40 * 1024

// DistillScenario asks the LLM to distill a pasted troubleshooting
// conversation into a reusable diagnosis scenario: the full SKILL.md content
// (frontmatter included), ready for preview and saving via SkillService.
// One-shot completion — no tools, non-streaming.
func (r *Runtime) DistillScenario(ctx context.Context, providerID, model, transcript string) (string, error) {
	ep, err := r.llm.ResolveLLM(providerID, model)
	if err != nil {
		return "", err
	}
	apiKey := ep.APIKey
	if apiKey == "" {
		apiKey = "local-no-key"
	}
	chatModel, err := openaimodel.NewChatModel(ctx, &openaimodel.ChatModelConfig{
		BaseURL: ep.BaseURL,
		APIKey:  apiKey,
		Model:   ep.Model,
	})
	if err != nil {
		return "", fmt.Errorf("create chat model: %w", err)
	}
	if len(transcript) > transcriptCharBudget {
		// Keep the tail — root cause and fix usually live in the last turns.
		transcript = "…[earlier turns omitted]\n" + transcript[len(transcript)-transcriptCharBudget:]
	}

	sys := "You distill an AI-assisted troubleshooting conversation into a reusable diagnosis scenario " +
		"for a playbook library (the user's team will reuse it whenever the same symptom appears).\n\n" +
		"Return ONLY the complete SKILL.md file content — no commentary, no wrapping code fence.\n" +
		"Structure:\n" +
		"1. A YAML frontmatter block: `name:` (lowercase kebab-case, letters/digits/'-', max 32 chars, " +
		"descriptive of the symptom, e.g. redis-conn-refused) and `description:` (one sentence describing the " +
		"symptom, in Chinese, used for matching).\n" +
		"2. A markdown body: a decision-tree troubleshooting guide for this scenario — numbered read-only steps " +
		"with exact POSIX commands (bounded output: head/tail/--no-pager), what each result implies, common root " +
		"causes, and a safety note that state-changing actions need user confirmation.\n\n" +
		"Rules: generalize — no hostnames, IPs, usernames or conversation-specific paths in commands; never " +
		"include secrets or tokens; keep the body under ~80 lines; narrate in Chinese, commands in English."

	resp, err := chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(sys),
		schema.UserMessage(transcript),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func (r *Runtime) buildResolver() CredsResolver {
	return func(sessionID string) (domain.Host, domain.Credentials, error) {
		host, ok := r.sshMgr.HostOfSession(sessionID)
		if !ok {
			return domain.Host{}, domain.Credentials{}, errors.New("session not found")
		}
		creds := domain.Credentials{}
		if r.secrets != nil {
			if v, err := r.secrets.GetHostSecret(host.ID, "password"); err == nil {
				creds.Password = string(v)
			}
			if v, err := r.secrets.GetHostSecret(host.ID, "passphrase"); err == nil {
				creds.KeyPassphrase = string(v)
			}
		}
		return host, creds, nil
	}
}
