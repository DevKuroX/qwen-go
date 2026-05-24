package rtk

import (
	"fmt"
	"sort"
	"strings"
)

// findFilter — port of 9router find.js.
func findFilter(input string) string {
	rawLines := strings.Split(input, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return input
	}

	byDir := make(map[string][]string)

	for _, path := range lines {
		slash := strings.LastIndexByte(path, '/')
		var dir, basename string
		if slash < 0 {
			dir = "."
			basename = path
		} else {
			dir = path[:slash]
			if dir == "" {
				dir = "/"
			}
			basename = path[slash+1:]
		}
		byDir[dir] = append(byDir[dir], basename)
	}

	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	var out strings.Builder
	fmt.Fprintf(&out, "%d files in %d dirs:\n\n", len(lines), len(dirs))

	showDirs := dirs
	if len(showDirs) > FindTotalDirMax {
		showDirs = showDirs[:FindTotalDirMax]
	}
	for _, dir := range showDirs {
		files := byDir[dir]
		fmt.Fprintf(&out, "%s/ (%d):\n", dir, len(files))
		show := files
		if len(show) > FindPerDirMax {
			show = show[:FindPerDirMax]
		}
		for _, f := range show {
			fmt.Fprintf(&out, "  %s\n", f)
		}
		if len(files) > FindPerDirMax {
			fmt.Fprintf(&out, "  +%d\n", len(files)-FindPerDirMax)
		}
		out.WriteString("\n")
	}
	if len(dirs) > FindTotalDirMax {
		fmt.Fprintf(&out, "+%d more dirs\n", len(dirs)-FindTotalDirMax)
	}

	return out.String()
}
