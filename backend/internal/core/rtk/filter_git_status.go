package rtk

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reLongBranch    = regexp.MustCompile(`^On branch (\S+)`)
	rePorcelainRow  = regexp.MustCompile(`^[ MADRCU?!][ MADRCU?!] `)
	rePorcelainHead = regexp.MustCompile(`^##\s*`)
	reLongStatus    = regexp.MustCompile(`^\s*(modified|new file|deleted|renamed|both modified):\s+(.+)$`)
)

// gitStatusFilter — port of 9router gitStatus.js.
func gitStatusFilter(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "") {
		return "Clean working tree"
	}

	branch := ""
	stagedFiles := make([]string, 0)
	modifiedFiles := make([]string, 0)
	untrackedFiles := make([]string, 0)
	staged := 0
	modified := 0
	untracked := 0
	conflicts := 0

	for _, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			continue
		}

		if m := reLongBranch.FindStringSubmatch(raw); m != nil {
			branch = m[1]
			continue
		}

		if strings.HasPrefix(raw, "##") {
			branch = rePorcelainHead.ReplaceAllString(raw, "")
			continue
		}

		if len(raw) >= 3 && rePorcelainRow.MatchString(raw) {
			x := raw[0]
			y := raw[1]
			file := raw[3:]

			if raw[:2] == "??" {
				untracked++
				untrackedFiles = append(untrackedFiles, file)
				continue
			}

			if strings.ContainsRune("MADRC", rune(x)) {
				staged++
				stagedFiles = append(stagedFiles, file)
			} else if x == 'U' {
				conflicts++
			}

			if y == 'M' || y == 'D' {
				modified++
				modifiedFiles = append(modifiedFiles, file)
			}
			continue
		}

		if m := reLongStatus.FindStringSubmatch(raw); m != nil {
			kind := m[1]
			path := strings.TrimSpace(m[2])
			switch kind {
			case "both modified":
				conflicts++
			case "modified", "deleted":
				modified++
				modifiedFiles = append(modifiedFiles, path)
			case "new file", "renamed":
				staged++
				stagedFiles = append(stagedFiles, path)
			}
			continue
		}
	}

	var out strings.Builder
	if branch != "" {
		fmt.Fprintf(&out, "* %s\n", branch)
	}

	writeBucket := func(label, marker string, count int, files []string, cap int) {
		if count == 0 {
			return
		}
		fmt.Fprintf(&out, "%s %s: %d files\n", marker, label, count)
		shown := files
		if len(shown) > cap {
			shown = shown[:cap]
		}
		for _, f := range shown {
			fmt.Fprintf(&out, "   %s\n", f)
		}
		if len(files) > cap {
			fmt.Fprintf(&out, "   ... +%d more\n", len(files)-cap)
		}
	}

	writeBucket("Staged", "+", staged, stagedFiles, StatusMaxFiles)
	writeBucket("Modified", "~", modified, modifiedFiles, StatusMaxFiles)
	writeBucket("Untracked", "?", untracked, untrackedFiles, StatusMaxUntrack)

	if conflicts > 0 {
		fmt.Fprintf(&out, "conflicts: %d files\n", conflicts)
	}

	if staged == 0 && modified == 0 && untracked == 0 && conflicts == 0 {
		out.WriteString("clean — nothing to commit\n")
	}

	return strings.TrimRight(out.String(), "\n")
}
