package tools

import (
	"fmt"
	"unicode/utf8"
)

// maxToolOutput is the default bound on how much text one tool invocation
// may return into the LLM conversation. Without it a `cat` of a large log or
// an unbounded `docker logs` can blow up the context window and kill the
// chat. The effective limit is per ToolSet (Deps.OutputLimitBytes, fed from
// the global agent settings).
const maxToolOutput = 64 << 10 // 64 KB

// capOutputAt truncates s to max, keeping the head and the tail (the start
// usually carries the error, the end the latest lines) and marking the
// elided middle. Cuts are rune-boundary safe. max <= 0 falls back to the
// package default.
func capOutputAt(s string, max int) string {
	if max <= 0 {
		max = maxToolOutput
	}
	if len(s) <= max {
		return s
	}
	keep := max / 2

	// Back the head cut up to a rune boundary.
	headEnd := keep
	for headEnd > 0 && !utf8.RuneStart(s[headEnd]) {
		headEnd--
	}
	// Advance the tail cut to a rune boundary.
	tailStart := len(s) - keep
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}

	omitted := tailStart - headEnd
	return s[:headEnd] +
		fmt.Sprintf("\n…[output truncated: %d bytes omitted]…\n", omitted) +
		s[tailStart:]
}
