package rtk

import (
	"fmt"
	"strings"
)

// gitDiffFilter compacts unified diff: file headers, hunk-level truncation at
// GitDiffHunkMaxLines, +/-/context counting. Port of 9router gitDiff.js.
func gitDiffFilter(diff string) string {
	const maxLines = 500
	maxHunkLines := GitDiffHunkMaxLines

	var result []string
	currentFile := ""
	added := 0
	removed := 0
	inHunk := false
	hunkShown := 0
	hunkSkipped := 0
	wasTruncated := false

	lines := strings.Split(diff, "\n")

	flushSkipped := func() {
		if hunkSkipped > 0 {
			result = append(result, fmt.Sprintf("  ... (%d lines truncated)", hunkSkipped))
			wasTruncated = true
			hunkSkipped = 0
		}
	}

outer:
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git"):
			flushSkipped()
			if currentFile != "" && (added > 0 || removed > 0) {
				result = append(result, fmt.Sprintf("  +%d -%d", added, removed))
			}
			parts := strings.SplitN(line, " b/", 2)
			if len(parts) > 1 {
				currentFile = parts[1]
			} else {
				currentFile = "unknown"
			}
			result = append(result, "\n"+currentFile)
			added = 0
			removed = 0
			inHunk = false
			hunkShown = 0
		case strings.HasPrefix(line, "@@"):
			flushSkipped()
			inHunk = true
			hunkShown = 0
			result = append(result, "  "+line)
		case inHunk:
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				added++
				if hunkShown < maxHunkLines {
					result = append(result, "  "+line)
					hunkShown++
				} else {
					hunkSkipped++
				}
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				removed++
				if hunkShown < maxHunkLines {
					result = append(result, "  "+line)
					hunkShown++
				} else {
					hunkSkipped++
				}
			} else if hunkShown < maxHunkLines && !strings.HasPrefix(line, "\\") {
				if hunkShown > 0 {
					result = append(result, "  "+line)
					hunkShown++
				}
			}
		}

		if len(result) >= maxLines {
			result = append(result, "\n... (more changes truncated)")
			wasTruncated = true
			break outer
		}
	}

	flushSkipped()

	if currentFile != "" && (added > 0 || removed > 0) {
		result = append(result, fmt.Sprintf("  +%d -%d", added, removed))
	}

	if wasTruncated {
		result = append(result, "[full diff: rtk git diff --no-compact]")
	}

	return strings.Join(result, "\n")
}
