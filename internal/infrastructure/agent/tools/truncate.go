package tools

import (
	"fmt"
	"unicode/utf8"
)

// maxToolOutput bounds how much text one tool invocation may return into the
// LLM conversation. Without it a `cat` of a large log or an unbounded
// `docker logs` can blow up the context window and kill the chat.
const maxToolOutput = 64 << 10 // 64 KB

// capOutput truncates s to maxToolOutput, keeping the head and the tail (the
// start usually carries the error, the end the latest lines) and marking the
// elided middle. Cuts are rune-boundary safe.
func capOutput(s string) string {
	if len(s) <= maxToolOutput {
		return s
	}
	const keep = maxToolOutput / 2

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
