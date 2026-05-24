package rtk

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var reLsDate = regexp.MustCompile(`\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2}\s+(\d{4}|\d{2}:\d{2})\s+`)

type lsRow struct {
	fileType byte
	size     int64
	name     string
}

func humanSize(b int64) string {
	switch {
	case b >= 1_048_576:
		return fmt.Sprintf("%.1fM", float64(b)/1_048_576)
	case b >= 1024:
		return fmt.Sprintf("%.1fK", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func parseLsLine(line string) *lsRow {
	loc := reLsDate.FindStringIndex(line)
	if loc == nil {
		return nil
	}
	name := line[loc[1]:]
	beforeDate := line[:loc[0]]
	fields := strings.Fields(beforeDate)
	if len(fields) < 4 {
		return nil
	}

	perms := fields[0]
	if perms == "" {
		return nil
	}
	fileType := perms[0]

	var size int64
	for i := len(fields) - 1; i >= 0; i-- {
		n, err := strconv.ParseInt(fields[i], 10, 64)
		if err != nil {
			continue
		}
		if strconv.FormatInt(n, 10) == fields[i] {
			size = n
			break
		}
	}
	return &lsRow{fileType: fileType, size: size, name: name}
}

// lsFilter — port of 9router ls.js.
func lsFilter(input string) string {
	dirs := make([]string, 0)
	type fileEntry struct {
		name string
		size string
	}
	files := make([]fileEntry, 0)
	byExt := make(map[string]int)

	for _, line := range strings.Split(input, "\n") {
		if strings.HasPrefix(line, "total ") || line == "" {
			continue
		}
		parsed := parseLsLine(line)
		if parsed == nil {
			continue
		}
		if parsed.name == "." || parsed.name == ".." {
			continue
		}
		if _, noise := LsNoiseDirs[parsed.name]; noise {
			continue
		}

		switch parsed.fileType {
		case 'd':
			dirs = append(dirs, parsed.name)
		case '-', 'l':
			dot := strings.LastIndexByte(parsed.name, '.')
			ext := "no ext"
			if dot > 0 {
				ext = parsed.name[dot:]
			}
			byExt[ext]++
			files = append(files, fileEntry{name: parsed.name, size: humanSize(parsed.size)})
		}
	}

	if len(dirs) == 0 && len(files) == 0 {
		return input
	}

	var out strings.Builder
	for _, d := range dirs {
		fmt.Fprintf(&out, "%s/\n", d)
	}
	for _, f := range files {
		fmt.Fprintf(&out, "%s  %s\n", f.name, f.size)
	}

	summary := fmt.Sprintf("\nSummary: %d files, %d dirs", len(files), len(dirs))
	if len(byExt) > 0 {
		type extCount struct {
			ext   string
			count int
		}
		extList := make([]extCount, 0, len(byExt))
		for e, c := range byExt {
			extList = append(extList, extCount{ext: e, count: c})
		}
		sort.Slice(extList, func(i, j int) bool {
			return extList[i].count > extList[j].count
		})
		show := extList
		if len(show) > LsExtSummaryTop {
			show = show[:LsExtSummaryTop]
		}
		parts := make([]string, 0, len(show))
		for _, ec := range show {
			parts = append(parts, fmt.Sprintf("%d %s", ec.count, ec.ext))
		}
		summary += " (" + strings.Join(parts, ", ")
		if len(extList) > LsExtSummaryTop {
			summary += fmt.Sprintf(", +%d more", len(extList)-LsExtSummaryTop)
		}
		summary += ")"
	}

	out.WriteString(summary)
	return out.String()
}
