package agent

import (
	"io"
	"testing"

	"github.com/cloudwego/eino/schema"
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
