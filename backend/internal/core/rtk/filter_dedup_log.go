package rtk

import (
	"fmt"
	"strings"
)

// dedupLogFilter — port of 9router dedupLog.js. Collapses consecutive duplicate
// lines + blank-line dedupe + hard line cap.
func dedupLogFilter(input string) string {
	lines := strings.Split(input, "\n")
	out := make([]string, 0, len(lines))
	prev := ""
	hasPrev := false
	runCount := 0
	blankStreak := 0

	flushRun := func() {
		if hasPrev && runCount > 1 {
			out = append(out, fmt.Sprintf("  ... (%d duplicate lines)", runCount-1))
		}
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if blankStreak < 1 {
				out = append(out, line)
			}
			blankStreak++
			flushRun()
			prev = ""
			hasPrev = false
			runCount = 0
			continue
		}
		blankStreak = 0
		if hasPrev && line == prev {
			runCount++
			continue
		}
		flushRun()
		out = append(out, line)
		prev = line
		hasPrev = true
		runCount = 1
		if len(out) >= DedupLineMax {
			out = append(out, fmt.Sprintf("... (truncated at %d lines)", DedupLineMax))
			return strings.Join(out, "\n")
		}
	}
	flushRun()
	return strings.Join(out, "\n")
}
