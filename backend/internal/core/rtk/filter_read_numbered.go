package rtk

import (
	"fmt"
	"strings"
)

// readNumberedFilter — port of 9router readNumbered.js. Smart-truncate over
// numbered-line file reads ("  1|content\n  2|content").
func readNumberedFilter(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) < SmartTruncateMinLines {
		return input
	}

	head := lines[:SmartTruncateHead]
	tail := lines[len(lines)-SmartTruncateTail:]
	cut := len(lines) - len(head) - len(tail)

	out := make([]string, 0, len(head)+1+len(tail))
	out = append(out, head...)
	out = append(out, fmt.Sprintf("... +%d lines truncated (file continues)", cut))
	out = append(out, tail...)
	return strings.Join(out, "\n")
}
