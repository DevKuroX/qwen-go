package rtk

import (
	"fmt"
	"sort"
	"strings"
)

// searchListFilter — port of 9router searchList.js. Compacts Cursor Glob output.
func searchListFilter(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) == 0 {
		return input
	}

	header := lines[0]
	rest := lines[1:]

	paths := make([]string, 0, len(rest))
	for _, raw := range rest {
		t := strings.TrimSpace(raw)
		if !strings.HasPrefix(t, "- ") {
			continue
		}
		paths = append(paths, t[2:])
	}
	if len(paths) == 0 {
		return input
	}

	byDir := make(map[string][]string)
	for _, p := range paths {
		slash := strings.LastIndexByte(p, '/')
		var dir, name string
		if slash < 0 {
			dir = "."
			name = p
		} else {
			dir = p[:slash]
			if dir == "" {
				dir = "/"
			}
			name = p[slash+1:]
		}
		byDir[dir] = append(byDir[dir], name)
	}

	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	var out strings.Builder
	fmt.Fprintf(&out, "%s\n%d files in %d dirs:\n\n", header, len(paths), len(dirs))

	showDirs := dirs
	if len(showDirs) > SearchListTotalDirMax {
		showDirs = showDirs[:SearchListTotalDirMax]
	}
	for _, dir := range showDirs {
		names := byDir[dir]
		fmt.Fprintf(&out, "%s/ (%d):\n", dir, len(names))
		show := names
		if len(show) > SearchListPerDirMax {
			show = show[:SearchListPerDirMax]
		}
		for _, n := range show {
			fmt.Fprintf(&out, "  %s\n", n)
		}
		if len(names) > SearchListPerDirMax {
			fmt.Fprintf(&out, "  +%d\n", len(names)-SearchListPerDirMax)
		}
		out.WriteString("\n")
	}
	if len(dirs) > SearchListTotalDirMax {
		fmt.Fprintf(&out, "+%d more dirs\n", len(dirs)-SearchListTotalDirMax)
	}

	return strings.TrimRight(out.String(), "\n")
}
