package rtk

import (
	"regexp"
	"strings"
)

var (
	reGitDiff     = regexp.MustCompile(`(?m)^diff --git `)
	reGitDiffHunk = regexp.MustCompile(`(?m)^@@ `)
	reGitStatus   = regexp.MustCompile(`(?m)^On branch |^nothing to commit|^Changes (not |to be )|^Untracked files:`)
	rePorcelain   = regexp.MustCompile(`(?m)^[ MADRCU?!][ MADRCU?!] \S`)
	reTreeGlyph   = regexp.MustCompile(`[├└]──|│  `)
	reLsRow       = regexp.MustCompile(`(?m)^[-dlbcps][rwx-]{9}`)
	reLsTotal     = regexp.MustCompile(`(?m)^total \d+$`)

	// readNumbered and searchList expose their detection regexes through
	// these vars so autodetect can probe without importing the filter
	// package back into a cycle.
	reReadNumbered     = regexp.MustCompile(`^\s*\d+\|`)
	reSearchListHeader = regexp.MustCompile(`^Result of search in '[^']*' \(total \d+ files?\):`)

	reLineNoOnly = regexp.MustCompile(`^\d+$`)
)

// AutoDetect mirrors 9router autodetect.js: peek the first DETECT_WINDOW
// bytes, then walk a strict priority chain. Returns a Filter with a nil
// Apply when no detector matched (caller treats as passthrough).
func AutoDetect(text string) Filter {
	head := text
	if len(head) > DetectWindow {
		head = head[:DetectWindow]
	}

	if reGitDiff.MatchString(head) || reGitDiffHunk.MatchString(head) {
		return registry[NameGitDiff]
	}
	if reGitStatus.MatchString(head) || isMostlyPorcelain(head) {
		return registry[NameGitStatus]
	}

	lines := strings.Split(head, "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}

	first5 := nonEmpty
	if len(first5) > 5 {
		first5 = first5[:5]
	}
	for _, l := range first5 {
		if isGrepLine(l) {
			return registry[NameGrep]
		}
	}

	if len(nonEmpty) >= 3 {
		all := true
		for _, l := range nonEmpty {
			if !isPathLike(l) {
				all = false
				break
			}
		}
		if all {
			return registry[NameFind]
		}
	}

	if reTreeGlyph.MatchString(head) {
		return registry[NameTree]
	}

	if reLsTotal.MatchString(head) || countMatches(head, reLsRow) >= 3 {
		return registry[NameLs]
	}

	if reSearchListHeader.MatchString(head) {
		return registry[NameSearchList]
	}

	if len(lines) >= SmartTruncateMinLines && isLineNumbered(lines) {
		return registry[NameReadNumbered]
	}

	if len(nonEmpty) >= 5 {
		return registry[NameDedupLog]
	}

	if strings.Count(text, "\n")+1 >= SmartTruncateMinLines {
		return registry[NameSmartTruncate]
	}

	return Filter{}
}

func isGrepLine(line string) bool {
	first := strings.IndexByte(line, ':')
	if first < 0 {
		return false
	}
	rest := line[first+1:]
	second := strings.IndexByte(rest, ':')
	if second < 0 {
		return false
	}
	lineno := rest[:second]
	if lineno == "" {
		return false
	}
	return reLineNoOnly.MatchString(lineno)
}

func isPathLike(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	if strings.Contains(t, ":") {
		return false
	}
	return strings.HasPrefix(t, ".") || strings.HasPrefix(t, "/") || strings.Contains(t, "/")
}

func isMostlyPorcelain(head string) bool {
	lines := strings.Split(head, "\n")
	filtered := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			filtered = append(filtered, l)
		}
	}
	if len(filtered) < 3 {
		return false
	}
	hits := 0
	for _, l := range filtered {
		if rePorcelain.MatchString(l) {
			hits++
		}
	}
	return float64(hits)/float64(len(filtered)) >= 0.6
}

func isLineNumbered(lines []string) bool {
	hits := 0
	nonEmpty := 0
	sample := lines
	if len(sample) > 100 {
		sample = sample[:100]
	}
	for _, l := range sample {
		if len(l) == 0 {
			continue
		}
		nonEmpty++
		if reReadNumbered.MatchString(l) {
			hits++
		}
	}
	if nonEmpty < 5 {
		return false
	}
	return float64(hits)/float64(nonEmpty) >= ReadNumberedMinHitRatio
}

func countMatches(text string, re *regexp.Regexp) int {
	return len(re.FindAllStringIndex(text, -1))
}
