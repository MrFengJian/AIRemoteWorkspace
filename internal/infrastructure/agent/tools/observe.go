package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"github.com/ai-remote/workspace/internal/application"
)

// RunObserver reports tool invocations upward (to the UI via Wails events).
// stepID is generated per invocation — consumers pair start and end by it.
// This is the reliable source of tool-step events: it works for every model,
// independent of how the eino ReAct stream routes messages.
type RunObserver interface {
	OnToolStart(sessionID, stepID, toolName, args string)
	OnToolEnd(sessionID, stepID, result string)
}

// Tool-result budget: the same truncated text goes to the LLM context and the
// frontend event, so a huge `ps aux` or file read can't blow either up.
const (
	maxToolResult = 16 * 1024
	headKeep      = 12 * 1024
	tailKeep      = 4 * 1024
)

// truncateResult keeps the head and tail of oversized tool output.
func truncateResult(s string) string {
	if len(s) <= maxToolResult {
		return s
	}
	return s[:headKeep] +
		fmt.Sprintf("\n...[truncated %d bytes]...\n", len(s)-headKeep-tailKeep) +
		s[len(s)-tailKeep:]
}

// observedTool wraps an InvokableTool: it emits start/end events, truncates
// oversized results, and converts approval denials into informational tool
// results so the model can adapt its plan instead of the whole chat failing
// (a tool error would terminate the eino ReAct graph).
type observedTool struct {
	inner     tool.InvokableTool
	sessionID string
	name      string
	observer  RunObserver
}

var _ tool.InvokableTool = (*observedTool)(nil)

func (o *observedTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return o.inner.Info(ctx)
}

func (o *observedTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	stepID := uuid.NewString()
	if o.observer != nil {
		o.observer.OnToolStart(o.sessionID, stepID, o.name, args)
	}
	result, err := o.inner.InvokableRun(ctx, args, opts...)
	if err != nil {
		// User denied (or the approval timed out): report it as a normal tool
		// result so the model changes course rather than the chat dying.
		if errors.Is(err, application.ErrDenied) || errors.Is(err, application.ErrApprovalTimeout) {
			msg := "The user DENIED this operation. Do NOT retry it. " +
				"Summarize what you were about to do and propose an alternative."
			if o.observer != nil {
				o.observer.OnToolEnd(o.sessionID, stepID, msg)
			}
			return msg, nil
		}
		if o.observer != nil {
			detail := result
			if detail != "" {
				detail += "\n"
			}
			o.observer.OnToolEnd(o.sessionID, stepID, truncateResult(detail+"ERROR: "+err.Error()))
		}
		return "", err
	}
	result = truncateResult(result)
	if o.observer != nil {
		o.observer.OnToolEnd(o.sessionID, stepID, result)
	}
	return result, nil
}

// observe wraps a built tool for event reporting. Returns the tool unchanged
// when there is no observer or the tool is not invokable.
func observe(t tool.BaseTool, sessionID, name string, observer RunObserver) tool.BaseTool {
	if observer == nil {
		return t
	}
	it, ok := t.(tool.InvokableTool)
	if !ok {
		return t
	}
	return &observedTool{inner: it, sessionID: sessionID, name: name, observer: observer}
}
