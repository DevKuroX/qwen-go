package rtk

import (
	"fmt"
	"strings"
)

// treeFilter — port of 9router tree.js.
func treeFilter(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) == 0 {
		return input
	}

	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(line, "director") && strings.Contains(line, "file") {
			continue
		}
		if strings.TrimSpace(line) == "" && len(filtered) == 0 {
			continue
		}
		filtered = append(filtered, line)
	}

	for len(filtered) > 0 && strings.TrimSpace(filtered[len(filtered)-1]) == "" {
		filtered = filtered[:len(filtered)-1]
	}

	if len(filtered) > TreeMaxLines {
		cut := len(filtered) - TreeMaxLines
		return strings.Join(filtered[:TreeMaxLines], "\n") + fmt.Sprintf("\n... +%d more lines", cut)
	}

	return strings.Join(filtered, "\n")
}
