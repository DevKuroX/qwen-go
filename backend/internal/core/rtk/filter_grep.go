package rtk

import (
	"fmt"
	"sort"
	"strings"
)

type grepHit struct {
	lineNum string
	content string
}

// grepFilter — port of 9router grep.js.
func grepFilter(input string) string {
	byFile := make(map[string][]grepHit)
	total := 0

	for _, line := range strings.Split(input, "\n") {
		first := strings.IndexByte(line, ':')
		if first < 0 {
			continue
		}
		second := strings.IndexByte(line[first+1:], ':')
		if second < 0 {
			continue
		}
		second += first + 1

		file := line[:first]
		lineNum := line[first+1 : second]
		content := line[second+1:]

		if !reLineNoOnly.MatchString(lineNum) {
			continue
		}
		total++
		byFile[file] = append(byFile[file], grepHit{lineNum: lineNum, content: content})
	}

	if total == 0 {
		return input
	}

	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	var out strings.Builder
	fmt.Fprintf(&out, "%d matches in %dF:\n\n", total, len(files))

	for _, file := range files {
		matches := byFile[file]
		fmt.Fprintf(&out, "[file] %s (%d):\n", file, len(matches))
		show := matches
		if len(show) > GrepPerFileMax {
			show = show[:GrepPerFileMax]
		}
		for _, m := range show {
			fmt.Fprintf(&out, "  %4s: %s\n", m.lineNum, strings.TrimSpace(m.content))
		}
		if len(matches) > GrepPerFileMax {
			fmt.Fprintf(&out, "  +%d\n", len(matches)-GrepPerFileMax)
		}
		out.WriteString("\n")
	}

	return out.String()
}
