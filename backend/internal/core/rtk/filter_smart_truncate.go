package rtk

import (
	"fmt"
	"strings"
)

// smartTruncateFilter — port of 9router smartTruncate.js. Head + tail line
// keep, middle collapsed.
func smartTruncateFilter(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) < SmartTruncateMinLines {
		return input
	}

	head := lines[:SmartTruncateHead]
	tail := lines[len(lines)-SmartTruncateTail:]
	cut := len(lines) - len(head) - len(tail)

	out := make([]string, 0, len(head)+1+len(tail))
	out = append(out, head...)
	out = append(out, fmt.Sprintf("... +%d lines truncated", cut))
	out = append(out, tail...)
	return strings.Join(out, "\n")
}
